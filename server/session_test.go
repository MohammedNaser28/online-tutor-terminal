package main

import (
	"fmt"
	"testing"
)

func TestManager_NewSession(t *testing.T) {
	m := NewSessionManager(5)
	s, err := m.NewSession("42", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.StudentID != "42" {
		t.Errorf("expected student id 42, got %s", s.StudentID)
	}
	if s.State() != SessionPending {
		t.Errorf("expected pending state, got %s", s.State())
	}

	tok, ok := m.LookupByStudentID("42")
	if !ok || tok != s.Token {
		t.Errorf("expected to find student 42 by id")
	}

	if m.CountActive() != 1 {
		t.Errorf("expected 1 active session, got %d", m.CountActive())
	}
}

func TestManager_DuplicateStudentID(t *testing.T) {
	m := NewSessionManager(5)
	_, err := m.NewSession("42", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.NewSession("42", "")
	if err == nil {
		t.Fatal("expected error for duplicate student id")
	}
}

func TestManager_CapacityReached(t *testing.T) {
	m := NewSessionManager(2)
	_, err := m.NewSession("1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = m.NewSession("2", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.NewSession("3", "")
	if err == nil {
		t.Fatal("expected error when capacity is full")
	}
}

func TestManager_GetSession(t *testing.T) {
	m := NewSessionManager(5)
	s, _ := m.NewSession("42", "")

	got, ok := m.GetSession(s.Token)
	if !ok {
		t.Fatal("expected to find session by token")
	}
	if got.StudentID != "42" {
		t.Errorf("expected student 42, got %s", got.StudentID)
	}

	_, ok = m.GetSession("nonexistent")
	if ok {
		t.Fatal("expected not to find nonexistent session")
	}
}

func TestManager_RemoveSession(t *testing.T) {
	m := NewSessionManager(5)
	s, _ := m.NewSession("42", "")

	m.RemoveSession(s.Token)

	if m.CountActive() != 0 {
		t.Errorf("expected 0 active after removal, got %d", m.CountActive())
	}

	_, ok := m.LookupByStudentID("42")
	if ok {
		t.Error("expected student 42 to be removed from lookup")
	}
}

func TestManager_SetState_Transitions(t *testing.T) {
	m := NewSessionManager(5)
	s, _ := m.NewSession("42", "")

	if err := m.SetSessionState(s.Token, SessionActive); err != nil {
		t.Fatalf("expected active transition ok: %v", err)
	}
	if s.State() != SessionActive {
		t.Errorf("expected active state, got %s", s.State())
	}

	if err := m.SetSessionState(s.Token, SessionOrphaned); err != nil {
		t.Fatalf("expected orphaned transition ok: %v", err)
	}
	if s.State() != SessionOrphaned {
		t.Errorf("expected orphaned state, got %s", s.State())
	}

	if err := m.SetSessionState(s.Token, SessionActive); err != nil {
		t.Fatalf("expected re-activation ok: %v", err)
	}
	if s.State() != SessionActive {
		t.Errorf("expected active state, got %s", s.State())
	}

	if err := m.SetSessionState(s.Token, SessionClosed); err != nil {
		t.Fatalf("expected closed transition ok: %v", err)
	}
	if s.State() != SessionClosed {
		t.Errorf("expected closed state, got %s", s.State())
	}

	err := m.SetSessionState(s.Token, SessionActive)
	if err == nil {
		t.Fatal("expected error setting state on closed session")
	}
}

func TestManager_CountActive(t *testing.T) {
	m := NewSessionManager(5)
	m.NewSession("1", "")
	m.NewSession("2", "")
	m.NewSession("3", "")

	if c := m.CountActive(); c != 3 {
		t.Errorf("expected 3 active, got %d", c)
	}

	for _, s := range m.sessions {
		m.SetSessionState(s.Token, SessionClosed)
	}

	if c := m.CountActive(); c != 0 {
		t.Errorf("expected 0 active after closing all, got %d", c)
	}
}

func TestManager_ConcurrentSessions(t *testing.T) {
	m := NewSessionManager(10)
	const n = 50
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		i := i
		go func() {
			s, err := m.NewSession(fmt.Sprintf("%d", i), "")
			if err == nil {
				m.SetSessionState(s.Token, SessionActive)
			}
			errs <- err
		}()
	}

	successes := 0
	for i := 0; i < n; i++ {
		if err := <-errs; err == nil {
			successes++
		}
	}

	if successes != 10 {
		t.Errorf("expected 10 successful sessions (cap 10), got %d", successes)
	}
	if m.CountActive() != 10 {
		t.Errorf("expected 10 active sessions, got %d", m.CountActive())
	}
}
