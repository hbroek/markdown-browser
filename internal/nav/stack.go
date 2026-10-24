// Package nav keeps the back/forward stack of the current session.
package nav

// Stack is a browser-style history: visiting truncates the forward entries.
type Stack struct {
	urls []string
	pos  int // index of the current entry; -1 when empty
}

func NewStack() *Stack {
	return &Stack{pos: -1}
}

// Visit makes url the current entry and drops any forward entries.
// Revisiting the current URL (a reload) does not add an entry.
func (s *Stack) Visit(url string) {
	if s.pos >= 0 && s.urls[s.pos] == url {
		return
	}
	s.urls = append(s.urls[:s.pos+1], url)
	s.pos++
}

// Current returns the current URL, or "" when nothing has been visited.
func (s *Stack) Current() string {
	if s.pos < 0 {
		return ""
	}
	return s.urls[s.pos]
}

func (s *Stack) CanBack() bool    { return s.pos > 0 }
func (s *Stack) CanForward() bool { return s.pos >= 0 && s.pos < len(s.urls)-1 }

// Back moves one entry back and returns its URL.
func (s *Stack) Back() (string, bool) {
	if !s.CanBack() {
		return "", false
	}
	s.pos--
	return s.urls[s.pos], true
}

// Forward moves one entry forward and returns its URL.
func (s *Stack) Forward() (string, bool) {
	if !s.CanForward() {
		return "", false
	}
	s.pos++
	return s.urls[s.pos], true
}
