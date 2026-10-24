package fetch

import (
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxLogEntries = 200

// Header is one header line, kept in a slice so the order is stable.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Entry describes one request/response exchange (one hop for redirects).
type Entry struct {
	ID          int       `json:"id"`
	Time        time.Time `json:"time"`
	Method      string    `json:"method"`
	URL         string    `json:"url"`
	Status      int       `json:"status"`     // 0 when the request failed
	StatusText  string    `json:"statusText"` // e.g. "200 OK"
	Proto       string    `json:"proto"`
	ContentType string    `json:"contentType"`
	DurationMS  int64     `json:"durationMs"`
	Bytes       int64     `json:"bytes"`
	ReqHeaders  []Header  `json:"reqHeaders"`
	RespHeaders []Header  `json:"respHeaders"`
	Error       string    `json:"error"`
}

// Log keeps the most recent exchanges for the console.
type Log struct {
	// OnEntry, if set, is called (outside any lock) after each entry is added.
	OnEntry func(Entry)

	mu      sync.Mutex
	nextID  int
	entries []Entry
}

func NewLog() *Log { return &Log{} }

func (l *Log) newID() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nextID++
	return l.nextID
}

func (l *Log) add(e Entry) {
	l.mu.Lock()
	l.entries = append(l.entries, e)
	if len(l.entries) > maxLogEntries {
		l.entries = l.entries[len(l.entries)-maxLogEntries:]
	}
	cb := l.OnEntry
	l.mu.Unlock()
	if cb != nil {
		cb(e)
	}
}

// Entries returns a copy of the log, oldest first.
func (l *Log) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := append([]Entry{}, l.entries...)
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}

func (l *Log) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = nil
}

// logFile records a local file load.
func (l *Log) logFile(url string, size int64, start time.Time, err error) {
	e := Entry{
		ID: l.newID(), Time: start, Method: "GET", URL: url,
		DurationMS: time.Since(start).Milliseconds(), Bytes: size,
	}
	if err != nil {
		e.Error = err.Error()
	} else {
		e.Status, e.StatusText, e.Proto = http.StatusOK, "200 OK (local file)", "file"
	}
	l.add(e)
}

// redact hides secret values but keeps enough to tell what was sent.
func redact(name, value string) string {
	const hidden = "[redacted]"
	switch strings.ToLower(name) {
	case "authorization", "proxy-authorization":
		scheme, _, found := strings.Cut(value, " ")
		if found {
			return scheme + " " + hidden
		}
		return hidden
	case "cookie":
		parts := strings.Split(value, ";")
		for i, p := range parts {
			n, _, _ := strings.Cut(strings.TrimSpace(p), "=")
			parts[i] = n + "=" + hidden
		}
		return strings.Join(parts, "; ")
	case "set-cookie":
		first, attrs, hasAttrs := strings.Cut(value, ";")
		n, _, _ := strings.Cut(first, "=")
		if hasAttrs {
			return n + "=" + hidden + ";" + attrs
		}
		return n + "=" + hidden
	}
	return value
}

func headers(h http.Header) []Header {
	var out []Header
	for name, vals := range h {
		for _, v := range vals {
			out = append(out, Header{Name: name, Value: redact(name, v)})
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// loggingTransport records every exchange that goes through the client,
// including each redirect hop and the cookies the jar added.
type loggingTransport struct {
	base http.RoundTripper
	log  *Log
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	e := Entry{
		ID: t.log.newID(), Time: start, Method: req.Method, URL: req.URL.String(),
		ReqHeaders: headers(req.Header),
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		e.Error = err.Error()
		e.DurationMS = time.Since(start).Milliseconds()
		t.log.add(e)
		return nil, err
	}
	e.Status, e.StatusText, e.Proto = resp.StatusCode, resp.Status, resp.Proto
	e.ContentType = resp.Header.Get("Content-Type")
	e.RespHeaders = headers(resp.Header)
	resp.Body = &countingBody{ReadCloser: resp.Body, finish: func(n int64, err error) {
		e.Bytes = n
		e.DurationMS = time.Since(start).Milliseconds()
		if err != nil {
			e.Error = err.Error()
		}
		t.log.add(e)
	}}
	return resp, nil
}

// countingBody reports the body size once it is fully read or closed.
type countingBody struct {
	io.ReadCloser
	n      int64
	once   sync.Once
	finish func(n int64, err error)
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.n += int64(n)
	if err != nil {
		report := err
		if err == io.EOF {
			report = nil
		}
		b.once.Do(func() { b.finish(b.n, report) })
	}
	return n, err
}

func (b *countingBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() { b.finish(b.n, nil) })
	return err
}
