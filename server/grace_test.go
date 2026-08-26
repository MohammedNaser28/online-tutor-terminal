package main

import (
	"testing"
)

func TestGracePeriod_OrphanTransition(t *testing.T) {
	m := NewSessionManager(5)
	s, _ := m.NewSession("42", "")
	m.SetSessionState(s.Token, SessionActive)

	if err := m.SetSessionState(s.Token, SessionOrphaned); err != nil {
		t.Fatalf("expected orphan transition ok: %v", err)
	}
	if s.State() != SessionOrphaned {
		t.Errorf("expected orphaned state, got %s", s.State())
	}
}

func TestGracePeriod_ReconnectWithinWindow(t *testing.T) {
	m := NewSessionManager(5)
	s, _ := m.NewSession("42", "")
	m.SetSessionState(s.Token, SessionActive)

	m.SetSessionState(s.Token, SessionOrphaned)

	m.SetSessionState(s.Token, SessionActive)
	if s.State() != SessionActive {
		t.Errorf("expected active after reconnect, got %s", s.State())
	}
}

func TestGracePeriod_CloseOrphanedSession(t *testing.T) {
	m := NewSessionManager(5)
	s, _ := m.NewSession("42", "")
	m.SetSessionState(s.Token, SessionActive)
	m.SetSessionState(s.Token, SessionOrphaned)

	m.SetSessionState(s.Token, SessionClosed)
	s.Close()

	if m.CountActive() != 0 {
		t.Errorf("expected 0 active after closing, got %d", m.CountActive())
	}
	_, ok := m.GetSession(s.Token)
	if !ok {
		t.Error("session should still be in map after Close (RemoveSession needed)")
	}
}

func TestGracePeriod_FullFlow(t *testing.T) {
	m := NewSessionManager(5)

	s, err := m.NewSession("student1", "")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	if s.State() != SessionPending {
		t.Errorf("expected pending, got %s", s.State())
	}

	m.SetSessionState(s.Token, SessionActive)
	if s.State() != SessionActive {
		t.Errorf("expected active, got %s", s.State())
	}

	m.SetSessionState(s.Token, SessionOrphaned)
	if s.State() != SessionOrphaned {
		t.Errorf("expected orphaned, got %s", s.State())
	}

	m.SetSessionState(s.Token, SessionActive)
	if s.State() != SessionActive {
		t.Errorf("expected active after reconnect, got %s", s.State())
	}

	m.SetSessionState(s.Token, SessionOrphaned)
	m.SetSessionState(s.Token, SessionClosed)
	s.Close()

	if m.CountActive() != 0 {
		t.Errorf("expected 0 active, got %d", m.CountActive())
	}
}

func TestGracePeriod_MultipleDisconnects(t *testing.T) {
	m := NewSessionManager(5)
	s, _ := m.NewSession("42", "")
	m.SetSessionState(s.Token, SessionActive)

	for i := 0; i < 5; i++ {
		m.SetSessionState(s.Token, SessionOrphaned)
		if s.State() != SessionOrphaned {
			t.Fatalf("iteration %d: expected orphaned, got %s", i, s.State())
		}
		m.SetSessionState(s.Token, SessionActive)
		if s.State() != SessionActive {
			t.Fatalf("iteration %d: expected active, got %s", i, s.State())
		}
	}
}

func TestGracePeriod_ReconnectAfterCleanup(t *testing.T) {
	m := NewSessionManager(5)
	s, _ := m.NewSession("42", "")
	m.SetSessionState(s.Token, SessionActive)
	m.SetSessionState(s.Token, SessionClosed)
	s.Close()
	m.RemoveSession(s.Token)

	if m.CountActive() != 0 {
		t.Errorf("expected 0 active, got %d", m.CountActive())
	}

	newS, err := m.NewSession("42", "")
	if err != nil {
		t.Fatalf("should allow new session after old is closed: %v", err)
	}
	if newS.Token == s.Token {
		t.Error("expected different token for new session")
	}
}

func TestGracePeriod_SameStudentReconnects(t *testing.T) {
	m := NewSessionManager(5)
	s, _ := m.NewSession("42", "")
	m.SetSessionState(s.Token, SessionActive)

	existingToken, ok := m.LookupByStudentID("42")
	if !ok {
		t.Fatal("expected to find student 42")
	}
	if existingToken != s.Token {
		t.Fatal("expected same token")
	}

	second, err := m.NewSession("42", "")
	if err == nil {
		t.Fatalf("expected error for duplicate, got session %s", second.Token)
	}
	if err.Error() != "student 42 already has an active session" {
		t.Errorf("unexpected error: %v", err)
	}
}
