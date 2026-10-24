package nav

import "testing"

func TestStack(t *testing.T) {
	s := NewStack()
	if s.CanBack() || s.CanForward() || s.Current() != "" {
		t.Fatal("new stack should be empty")
	}
	if _, ok := s.Back(); ok {
		t.Error("Back on empty stack")
	}

	s.Visit("a")
	s.Visit("b")
	s.Visit("c")
	if s.Current() != "c" || !s.CanBack() || s.CanForward() {
		t.Fatalf("after visits: current=%q", s.Current())
	}

	if u, ok := s.Back(); !ok || u != "b" {
		t.Errorf("Back = %q, %v", u, ok)
	}
	if !s.CanForward() {
		t.Error("expected CanForward")
	}

	s.Visit("d") // truncates "c"
	if s.CanForward() {
		t.Error("forward entries should be dropped")
	}
	if u, _ := s.Back(); u != "b" {
		t.Errorf("Back = %q, want b", u)
	}
	if u, _ := s.Back(); u != "a" {
		t.Errorf("Back = %q, want a", u)
	}
	if s.CanBack() {
		t.Error("at start")
	}
	if u, ok := s.Forward(); !ok || u != "b" {
		t.Errorf("Forward = %q, %v", u, ok)
	}
}

func TestVisitSameURLIsNoop(t *testing.T) {
	s := NewStack()
	s.Visit("a")
	s.Visit("a")
	if s.CanBack() {
		t.Error("duplicate visit added an entry")
	}
}
