// Package page defines the data handed from the Go backend to the frontend.
package page

// Kind says how the frontend should display a Page.
type Kind string

const (
	KindMarkdown Kind = "markdown"
	KindFallback Kind = "fallback"
	KindError    Kind = "error"
)

// Link is a hyperlink found on a page.
type Link struct {
	Text string `json:"text"`
	Href string `json:"href"`
}

// Page is one loaded document.
type Page struct {
	URL         string `json:"url"`
	Kind        Kind   `json:"kind"`
	Status      int    `json:"status"`
	ContentType string `json:"contentType"`
	Title       string `json:"title"`
	HTML        string `json:"html"`   // sanitized, links resolved (markdown only)
	Source      string `json:"source"` // raw markdown (markdown only)
	Links       []Link `json:"links"`  // fallback only
	Error       string `json:"error"`

	Origin     string `json:"origin"`     // set on 401/403 so the UI can offer credentials
	AuthScheme string `json:"authScheme"` // "basic", "bearer", ... from WWW-Authenticate

	CanBack    bool `json:"canBack"`
	CanForward bool `json:"canForward"`
}
