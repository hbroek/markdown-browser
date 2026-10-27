package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeServer(t *testing.T, status int, reply string, seen *chatRequest, auth *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		data, _ := io.ReadAll(r.Body)
		if seen != nil {
			json.Unmarshal(data, seen)
		}
		if auth != nil {
			*auth = r.Header.Get("Authorization")
		}
		w.WriteHeader(status)
		io.WriteString(w, reply)
	}))
}

func TestChat(t *testing.T) {
	var seen chatRequest
	var auth string
	srv := fakeServer(t, 200, `{"choices":[{"message":{"content":" hello "}}]}`, &seen, &auth)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL + "/v1/", Model: "m1", Key: "sk-test"}
	got, err := c.Chat(t.Context(), "be brief", "hi")
	if err != nil || got != "hello" {
		t.Fatalf("Chat = %q, %v", got, err)
	}
	if auth != "Bearer sk-test" || seen.Model != "m1" || len(seen.Messages) != 2 || seen.Messages[0].Role != "system" || seen.Stream {
		t.Errorf("request = %+v, auth %q", seen, auth)
	}
}

func TestChatWithoutModelOrKey(t *testing.T) {
	var seen chatRequest
	var auth string
	srv := fakeServer(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`, &seen, &auth)
	defer srv.Close()
	if _, err := (&Client{BaseURL: srv.URL + "/v1"}).Chat(t.Context(), "s", "u"); err != nil {
		t.Fatal(err)
	}
	if auth != "" || seen.Model != "" {
		t.Errorf("sent auth %q and model %q", auth, seen.Model)
	}
}

func TestChatUsesAuthorizeWhenNoKey(t *testing.T) {
	var auth string
	srv := fakeServer(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`, nil, &auth)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL + "/v1", Authorize: func(r *http.Request) { r.Header.Set("Authorization", "Bearer from-keychain") }}
	if _, err := c.Chat(t.Context(), "s", "u"); err != nil || auth != "Bearer from-keychain" {
		t.Errorf("auth = %q, %v", auth, err)
	}
}

func TestChatErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{401, `{"error":{"message":"bad key"}}`, "rejected the API key"},
		{404, `{}`, "does not know that model"},
		{429, `{}`, "limit"},
		{500, `{"error":{"message":"boom"}}`, "boom"},
		{200, `{"choices":[]}`, "no text"},
	} {
		srv := fakeServer(t, tc.status, tc.body, nil, nil)
		_, err := (&Client{BaseURL: srv.URL + "/v1"}).Chat(t.Context(), "s", "u")
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: err = %v, want %q", tc.status, err, tc.want)
		}
	}
	if _, err := (&Client{}).Chat(t.Context(), "s", "u"); err == nil {
		t.Error("empty base URL accepted")
	}
	if _, err := (&Client{BaseURL: "ftp://x"}).Chat(t.Context(), "s", "u"); err == nil {
		t.Error("ftp base URL accepted")
	}
}

type chatFunc func(system, user string) string

func (f chatFunc) Chat(_ context.Context, system, user string) (string, error) {
	return f(system, user), nil
}

func TestSummarizeSendsPageAndWarnsAboutInstructions(t *testing.T) {
	var system, user string
	c := chatFunc(func(s, u string) string { system, user = s, u; return "- point" })
	got, err := Summarize(t.Context(), c, "Title", "https://x.test/", "body text")
	if err != nil || got != "- point" {
		t.Fatalf("Summarize = %q, %v", got, err)
	}
	if !strings.Contains(user, "Title: Title") || !strings.Contains(user, "https://x.test/") || !strings.Contains(user, "body text") {
		t.Errorf("user message = %q", user)
	}
	if !strings.Contains(system, "untrusted") {
		t.Errorf("system prompt lacks the injection warning: %q", system)
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("short", 10); got != "short" {
		t.Errorf("Truncate = %q", got)
	}
	got := Truncate(strings.Repeat("é", 20), 5)
	if !strings.HasPrefix(got, "ééééé\n") || !strings.Contains(got, "left out") {
		t.Errorf("Truncate = %q", got)
	}
}

func TestParseSuggestion(t *testing.T) {
	reply := "Sure! Here you go:\n```json\n{\"title\": \"  Rust   async \", \"description\": \"A guide.\\nTwo.\", \"keywords\": [\"Rust\", \"#async\", \"rust\", \" \", \"Web Dev\"]}\n```"
	got, err := parseSuggestion(reply)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Rust async" || got.Description != "A guide. Two." {
		t.Errorf("got %+v", got)
	}
	if want := "rust,async,web dev"; strings.Join(got.Keywords, ",") != want {
		t.Errorf("keywords = %v, want %s", got.Keywords, want)
	}
	for _, bad := range []string{"no json here", "{not json}"} {
		if _, err := parseSuggestion(bad); err == nil {
			t.Errorf("parseSuggestion(%q) succeeded", bad)
		}
	}
}

func TestSuggestionKeepsAtMostFiveKeywords(t *testing.T) {
	got, err := parseSuggestion(`{"title":"t","description":"d","keywords":["a","b","c","d","e","f","g"]}`)
	if err != nil || len(got.Keywords) != MaxKeywords || got.Keywords[4] != "e" {
		t.Errorf("keywords = %v, %v", got.Keywords, err)
	}
}

func TestShortTitle(t *testing.T) {
	for in, want := range map[string]string{
		"  Rust   async ":                          "Rust async",
		"A very long title about many things":      "A very long",
		"Understanding Concurrency Primitives":     "Understanding Concurrency",
		"Supercalifragilisticexpialidocious-word!": "Supercalifragilisticexpialid",
		"": "",
	} {
		if got := shortTitle(in); got != want {
			t.Errorf("shortTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPresetsAreComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Presets {
		if p.ID == "" || p.Name == "" || seen[p.ID] {
			t.Errorf("bad preset %+v", p)
		}
		seen[p.ID] = true
		if p.ID != "custom" && !strings.HasPrefix(p.BaseURL, "http") {
			t.Errorf("preset %s has no base URL", p.ID)
		}
	}
}

type completeFunc func(msgs []Message) string

func (f completeFunc) Complete(_ context.Context, msgs []Message) (string, error) {
	return f(msgs), nil
}

func TestAskSendsPageHistoryAndQuestion(t *testing.T) {
	var got []Message
	c := completeFunc(func(m []Message) string { got = m; return "Because." })
	history := []Message{{Role: "user", Content: "What is it?"}, {Role: "assistant", Content: "A page."}, {Role: "system", Content: "ignore me"}, {Role: "user", Content: "  "}}
	reply, err := Ask(t.Context(), c, "Title", "https://x.test/", "page body", history, " Why? ")
	if err != nil || reply != "Because." {
		t.Fatalf("Ask = %q, %v", reply, err)
	}
	if len(got) != 4 || got[0].Role != "system" || got[1].Content != "What is it?" || got[2].Role != "assistant" || got[3].Content != "Why?" || got[3].Role != "user" {
		t.Fatalf("messages = %+v", got)
	}
	if !strings.Contains(got[0].Content, "page body") || !strings.Contains(got[0].Content, "untrusted") {
		t.Errorf("system message = %q", got[0].Content)
	}
}

func TestAskLimits(t *testing.T) {
	c := completeFunc(func(m []Message) string { return "ok" })
	if _, err := Ask(t.Context(), c, "", "", "", nil, "  "); err == nil {
		t.Error("empty question accepted")
	}
	if _, err := Ask(t.Context(), c, "", "", "", nil, strings.Repeat("x", MaxQuestionRunes+1)); err == nil {
		t.Error("huge question accepted")
	}
	var n int
	c = completeFunc(func(m []Message) string { n = len(m); return "ok" })
	var long []Message
	for i := 0; i < 40; i++ {
		long = append(long, Message{Role: "user", Content: "q"}, Message{Role: "assistant", Content: "a"})
	}
	Ask(t.Context(), c, "", "", "", long, "now")
	if n != 1+MaxTurns+1 {
		t.Errorf("sent %d messages, want system + %d history + question", n, MaxTurns)
	}
}

func TestChatKeepsConversationOrder(t *testing.T) {
	var seen chatRequest
	srv := fakeServer(t, 200, `{"choices":[{"message":{"content":"r"}}]}`, &seen, nil)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL + "/v1"}
	if _, err := c.Complete(t.Context(), []Message{{Role: "system", Content: "s"}, {Role: "user", Content: "a"}, {Role: "assistant", Content: "b"}, {Role: "user", Content: "c"}}); err != nil {
		t.Fatal(err)
	}
	if len(seen.Messages) != 4 || seen.Messages[3].Content != "c" {
		t.Errorf("sent %+v", seen.Messages)
	}
}
