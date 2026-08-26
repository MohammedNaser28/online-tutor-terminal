package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestShutdown_RejectsNewJoins(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	s.shutdown = true

	body := strings.NewReader("name=student42&code=test-event")
	req := httptest.NewRequest("POST", "/join", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleJoin(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 during shutdown, got %d", w.Code)
	}
}

func TestShutdown_ServeHTTPRoutes(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	ts := httptest.NewServer(s)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for /, got %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/nonexistent")
	if err != nil {
		t.Fatalf("GET /nonexistent: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for /nonexistent, got %d", resp.StatusCode)
	}
}

func TestShutdown_ExistingSessionsPersist(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	s.manager.NewSession("student1", "")
	s.manager.NewSession("student2", "")

	if c := s.manager.CountActive(); c != 2 {
		t.Fatalf("expected 2 active sessions, got %d", c)
	}

	s.shutdown = true

	if c := s.manager.CountActive(); c != 2 {
		t.Errorf("expected 2 active sessions after shutdown flag, got %d", c)
	}
}

func TestShutdown_CleanupAllSessions(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	s1, _ := s.manager.NewSession("student1", "")
	s2, _ := s.manager.NewSession("student2", "")
	s.manager.SetSessionState(s1.Token, SessionActive)
	s.manager.SetSessionState(s2.Token, SessionActive)

	s1.Close()
	s2.Close()
	s.manager.RemoveSession(s1.Token)
	s.manager.RemoveSession(s2.Token)

	if c := s.manager.CountActive(); c != 0 {
		t.Errorf("expected 0 active after cleanup, got %d", c)
	}
}
