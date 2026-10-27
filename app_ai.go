package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"markdown-browser/internal/ai"
	"markdown-browser/internal/auth"
	"markdown-browser/internal/chats"
	"markdown-browser/internal/fetch"
	"markdown-browser/internal/render"
	"markdown-browser/internal/settings"
)

// AIInfo is what the settings dialog shows.
type AIInfo struct {
	Presets    []ai.Preset `json:"presets"`
	Preset     string      `json:"preset"`
	BaseURL    string      `json:"baseUrl"`
	Model      string      `json:"model"`
	Configured bool        `json:"configured"`
	HasKey     bool        `json:"hasKey"`
	Consented  bool        `json:"consented"`
}

// AIInput is a provider choice from the settings dialog.
type AIInput struct {
	Preset  string `json:"preset"`
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
	Consent bool   `json:"consent"` // page text may be sent to a server on the internet
}

// aiOrigin checks baseURL and says whether it is a server outside this computer.
func aiOrigin(baseURL string) (origin string, remote bool, err error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return "", false, fmt.Errorf("%q is not a valid address", baseURL)
	}
	origin, err = auth.NormalizeOrigin(u.String())
	if err != nil {
		return "", false, err
	}
	return origin, !auth.IsLoopbackHost(u.Hostname()), nil
}

func (a *App) aiHasKey(origin string) bool {
	for _, info := range a.auth.List() {
		if info.Origin == origin {
			return true
		}
	}
	return false
}

// aiContext is the context for one AI request. With an id from the page, CancelAI
// with the same id stops it, including the request to the model. Call done when finished.
func (a *App) aiContext(id string) (ctx context.Context, done func()) {
	ctx, cancel := context.WithCancel(a.bookmarkCtx())
	if id == "" {
		return ctx, cancel
	}
	a.aiMu.Lock()
	if a.aiCancels == nil {
		a.aiCancels = map[string]context.CancelFunc{}
	}
	a.aiCancels[id] = cancel
	a.aiMu.Unlock()
	return ctx, func() {
		cancel()
		a.aiMu.Lock()
		delete(a.aiCancels, id)
		a.aiMu.Unlock()
	}
}

// CancelAI stops the request that was started with this id, if it is still running.
func (a *App) CancelAI(id string) {
	a.aiMu.Lock()
	cancel := a.aiCancels[id]
	a.aiMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

var errCancelled = errors.New("cancelled")

// cancelled turns whatever went wrong into "cancelled" when the user stopped the request.
func cancelled(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil {
		return errCancelled
	}
	return err
}

// AISettings describes the current provider and the ones on offer.
func (a *App) AISettings() AIInfo {
	a.bmMu.RLock()
	cfg := a.settings.AI
	a.bmMu.RUnlock()
	info := AIInfo{Presets: ai.Presets, Preset: cfg.Preset, BaseURL: cfg.BaseURL, Model: cfg.Model, Configured: cfg.BaseURL != ""}
	if origin, remote, err := aiOrigin(cfg.BaseURL); err == nil {
		info.HasKey = a.aiHasKey(origin)
		info.Consented = !remote || cfg.Consent == origin
	}
	return info
}

// SaveAI stores the provider. A new API key goes to the keychain; an empty one
// keeps the key already saved for that server.
func (a *App) SaveAI(in AIInput, apiKey string) error {
	in.BaseURL = strings.TrimSpace(in.BaseURL)
	in.Model = strings.TrimSpace(in.Model)
	origin, remote, err := aiOrigin(in.BaseURL)
	if err != nil {
		return err
	}
	if remote && !in.Consent {
		return fmt.Errorf("allow sending page text to %s to use it", strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://"))
	}
	key := strings.TrimSpace(apiKey)
	for _, p := range ai.Presets {
		if p.ID == in.Preset && p.KeyRequired && key == "" && !a.aiHasKey(origin) {
			return fmt.Errorf("%s needs an API key", p.Name)
		}
	}
	if key != "" {
		if _, err := a.auth.Save(auth.Credential{Origin: origin, Kind: auth.Bearer, Token: key}); err != nil {
			return err
		}
	}
	cfg := settings.AI{Preset: in.Preset, BaseURL: in.BaseURL, Model: in.Model}
	if remote {
		cfg.Consent = origin
	}
	a.bmMu.Lock()
	a.settings.AI = cfg
	a.bmMu.Unlock()
	if a.settingsPath == "" {
		return nil
	}
	raw := settings.LoadRaw(a.settingsPath) // not the resolved copy, which would pin defaults
	raw.AI = cfg
	return settings.Save(a.settingsPath, raw)
}

// TestAI checks a provider with a one-word request, using the typed key or the saved one.
func (a *App) TestAI(in AIInput, apiKey string) (string, error) {
	c := &ai.Client{BaseURL: in.BaseURL, Model: in.Model, Key: strings.TrimSpace(apiKey), Authorize: a.auth.Apply, MaxTokens: 16}
	return c.Chat(a.bookmarkCtx(), "Answer with the single word OK.", "ping")
}

// aiClient is the configured provider, if the user has allowed using it.
func (a *App) aiClient() (*ai.Client, error) {
	info := a.AISettings()
	if !info.Configured {
		return nil, errors.New("no AI provider is set up yet (menu → AI assistant)")
	}
	if !info.Consented {
		return nil, errors.New("sending page text to this AI server is not allowed yet (menu → AI assistant)")
	}
	return &ai.Client{BaseURL: info.BaseURL, Model: info.Model, Authorize: a.auth.Apply}, nil
}

// pageCache remembers the text of recently read pages, so a conversation about
// a page does not fetch it again for every question.
type pageCacheEntry struct {
	title, text string
	at          time.Time
}

const (
	pageCacheAge  = 10 * time.Minute
	pageCacheSize = 8
)

// pageText returns the page's title and text, from the cache when it is fresh.
func (a *App) pageText(ctx context.Context, rawURL string) (title, text string, err error) {
	if strings.HasPrefix(rawURL, "file:") { // a local file may be edited at any moment
		return a.fetchPageText(ctx, rawURL)
	}
	a.pageMu.Lock()
	e, ok := a.pageCache[rawURL]
	a.pageMu.Unlock()
	if ok && time.Since(e.at) < pageCacheAge {
		return e.title, e.text, nil
	}
	title, text, err = a.fetchPageText(ctx, rawURL)
	if err != nil {
		return "", "", err
	}
	a.pageMu.Lock()
	defer a.pageMu.Unlock()
	if a.pageCache == nil {
		a.pageCache = map[string]pageCacheEntry{}
	}
	for len(a.pageCache) >= pageCacheSize { // drop the oldest
		oldest := ""
		for u, c := range a.pageCache {
			if oldest == "" || c.at.Before(a.pageCache[oldest].at) {
				oldest = u
			}
		}
		delete(a.pageCache, oldest)
	}
	a.pageCache[rawURL] = pageCacheEntry{title: title, text: text, at: time.Now()}
	return title, text, nil
}

// fetchPageText fetches a page the way the browser does and returns its title and plain text.
func (a *App) fetchPageText(ctx context.Context, rawURL string) (title, text string, err error) {
	if rawURL == "" || strings.HasPrefix(rawURL, "bookmarks:") {
		return "", "", errors.New("there is no page to read")
	}
	res, err := a.fetcher.Get(ctx, rawURL)
	if err != nil {
		return "", "", err
	}
	if res.Status >= 400 {
		return "", "", fmt.Errorf("the page answered %d", res.Status)
	}
	base, _ := url.Parse(res.URL)
	if render.IsFeed(res.Body) { // read the entries, not the XML around them
		if html, feedTitle, ok, err := render.Feed(res.Body, base); err == nil && ok {
			_, text = render.Text([]byte(html), "text/html")
			return feedTitle, text, nil
		}
	}
	if fetch.IsMarkdown(res.ContentType) {
		if _, t, err := render.Markdown(res.Body, base); err == nil {
			title = t
		}
		return title, string(res.Body), nil
	}
	title, text = render.Text(res.Body, res.ContentType)
	if strings.TrimSpace(text) == "" {
		return "", "", errors.New("the page has no text to read")
	}
	return title, text, nil
}

// AISummary is a page summary and who wrote it.
type AISummary struct {
	HTML   string `json:"html"`   // sanitized
	Source string `json:"source"` // the model, or the provider when no model is set
	Millis int64  `json:"millis"` // how long the model took to answer
}

// aiSource names what answers: the model, else the provider's name, else its host.
func (a *App) aiSource() string {
	info := a.AISettings()
	if m := strings.TrimSpace(info.Model); m != "" {
		return m
	}
	for _, p := range info.Presets {
		if p.ID == info.Preset && p.ID != "custom" {
			return p.Name
		}
	}
	if u, err := url.Parse(info.BaseURL); err == nil && u.Host != "" {
		return u.Host
	}
	return "an AI model"
}

// SummarizePage returns an AI summary of the page.
func (a *App) SummarizePage(requestID, rawURL string) (AISummary, error) {
	c, err := a.aiClient()
	if err != nil {
		return AISummary{}, err
	}
	ctx, done := a.aiContext(requestID)
	defer done()
	title, text, err := a.pageText(ctx, rawURL)
	if err != nil {
		return AISummary{}, cancelled(ctx, err)
	}
	started := time.Now()
	reply, err := ai.Summarize(ctx, c, title, rawURL, text)
	if err != nil {
		return AISummary{}, cancelled(ctx, err)
	}
	took := time.Since(started)
	base, _ := url.Parse(rawURL)
	html, _, err := render.Markdown([]byte(reply), base)
	return AISummary{HTML: html, Source: a.aiSource(), Millis: took.Milliseconds()}, err
}

// SuggestBookmark proposes a title, description and keywords for a page.
func (a *App) SuggestBookmark(requestID, rawURL string) (ai.Suggestion, error) {
	c, err := a.aiClient()
	if err != nil {
		return ai.Suggestion{}, err
	}
	ctx, done := a.aiContext(requestID)
	defer done()
	title, text, err := a.pageText(ctx, rawURL)
	if err != nil {
		return ai.Suggestion{}, cancelled(ctx, err)
	}
	s, err := ai.SuggestMetadata(ctx, c, title, rawURL, text)
	return s, cancelled(ctx, err)
}

// AIAnswer is one reply in a conversation about a page.
type AIAnswer struct {
	Text   string `json:"text"`   // as the model wrote it, to send back as history
	HTML   string `json:"html"`   // sanitized, for display
	Source string `json:"source"` // the model, or the provider when no model is set
	Millis int64  `json:"millis"` // how long the model took to answer
}

// AskPage answers a question about the page, continuing the conversation in history.
func (a *App) AskPage(requestID, rawURL string, history []ai.Message, question string) (AIAnswer, error) {
	c, err := a.aiClient()
	if err != nil {
		return AIAnswer{}, err
	}
	ctx, done := a.aiContext(requestID)
	defer done()
	title, text, err := a.pageText(ctx, rawURL)
	if err != nil {
		return AIAnswer{}, cancelled(ctx, err)
	}
	started := time.Now()
	reply, err := ai.Ask(ctx, c, title, rawURL, text, history, question)
	if err != nil {
		return AIAnswer{}, cancelled(ctx, err)
	}
	took := time.Since(started)
	base, _ := url.Parse(rawURL)
	html, _, err := render.Markdown([]byte(reply), base)
	return AIAnswer{Text: reply, HTML: html, Source: a.aiSource(), Millis: took.Milliseconds()}, err
}

// ChatTurn is one message of a saved conversation about a page.
type ChatTurn struct {
	Role   string `json:"role"` // "user" or "assistant"
	Text   string `json:"text"`
	HTML   string `json:"html"` // for an answer, made from Text when loaded
	Source string `json:"source"`
	Millis int64  `json:"millis"`
}

func (a *App) conversations() *chats.Store {
	a.pageMu.Lock()
	defer a.pageMu.Unlock()
	if a.chats == nil {
		a.chats = chats.Open("")
	}
	return a.chats
}

// SavedConversation returns the conversation saved for a page, answers rendered as sanitized HTML.
func (a *App) SavedConversation(rawURL string) []ChatTurn {
	base, _ := url.Parse(rawURL)
	var out []ChatTurn
	for _, t := range a.conversations().Get(rawURL) {
		turn := ChatTurn{Role: t.Role, Text: t.Text, Source: t.Source, Millis: t.Millis}
		if t.Role == "assistant" {
			turn.HTML, _, _ = render.Markdown([]byte(t.Text), base)
		}
		out = append(out, turn)
	}
	return out
}

// SaveConversation keeps the conversation for a page; an empty one forgets it.
func (a *App) SaveConversation(rawURL string, turns []ChatTurn) {
	saved := make([]chats.Turn, 0, len(turns))
	for _, t := range turns {
		saved = append(saved, chats.Turn{Role: t.Role, Text: t.Text, Source: t.Source, Millis: t.Millis})
	}
	a.conversations().Put(rawURL, saved)
}

// ForgetConversations deletes every saved conversation and returns how many pages had one.
func (a *App) ForgetConversations() int {
	n := a.conversations().Len()
	a.conversations().Clear()
	return n
}
