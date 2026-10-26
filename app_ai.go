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

// pageText fetches a page the way the browser does and returns its title and plain text.
func (a *App) pageText(ctx context.Context, rawURL string) (title, text string, err error) {
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
	if fetch.IsMarkdown(res.ContentType) || render.IsFeed(res.Body) {
		base, _ := url.Parse(res.URL)
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
func (a *App) SummarizePage(rawURL string) (AISummary, error) {
	c, err := a.aiClient()
	if err != nil {
		return AISummary{}, err
	}
	ctx := a.bookmarkCtx()
	title, text, err := a.pageText(ctx, rawURL)
	if err != nil {
		return AISummary{}, err
	}
	started := time.Now()
	reply, err := ai.Summarize(ctx, c, title, rawURL, text)
	if err != nil {
		return AISummary{}, err
	}
	took := time.Since(started)
	base, _ := url.Parse(rawURL)
	html, _, err := render.Markdown([]byte(reply), base)
	return AISummary{HTML: html, Source: a.aiSource(), Millis: took.Milliseconds()}, err
}

// SuggestBookmark proposes a title, description and keywords for a page.
func (a *App) SuggestBookmark(rawURL string) (ai.Suggestion, error) {
	c, err := a.aiClient()
	if err != nil {
		return ai.Suggestion{}, err
	}
	ctx := a.bookmarkCtx()
	title, text, err := a.pageText(ctx, rawURL)
	if err != nil {
		return ai.Suggestion{}, err
	}
	return ai.SuggestMetadata(ctx, c, title, rawURL, text)
}
