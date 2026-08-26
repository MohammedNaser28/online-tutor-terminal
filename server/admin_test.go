package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminState_ReturnsState(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	s.manager.NewSession("student1", "")
	s.manager.NewSession("student2", "")

	req := httptest.NewRequest("GET", "/admin/state", nil)
	req.Header.Set("X-Admin-Token", "admin-secret-123")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp adminStateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(resp.Sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(resp.Sessions))
	}
	if resp.MaxConcurrent != 2 {
		t.Errorf("expected max 2, got %d", resp.MaxConcurrent)
	}
	if resp.ShuttingDown {
		t.Error("expected not shutting down")
	}
}

func TestAdminKill_KillsSession(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	session, _ := s.manager.NewSession("target", "")
	token := session.Token

	req := httptest.NewRequest("POST", "/admin/kill?token="+token, nil)
	req.Header.Set("X-Admin-Token", "admin-secret-123")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "killed" {
		t.Errorf("expected status 'killed', got %q", resp["status"])
	}

	_, ok := s.manager.GetSession(token)
	if ok {
		t.Error("session should be removed after kill")
	}
}

func TestAdminKill_NotFound(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/admin/kill?token=nonexistent", nil)
	req.Header.Set("X-Admin-Token", "admin-secret-123")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminShutdown_SetsFlag(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/admin/shutdown", nil)
	req.Header.Set("X-Admin-Token", "admin-secret-123")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if !s.shutdown {
		t.Error("expected shutdown flag set")
	}
}

func TestQueue_AddRemove(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	s.addToQueue("alice")
	s.addToQueue("bob")
	s.addToQueue("alice") // duplicate — should be no-op

	if len(s.queue) != 2 {
		t.Fatalf("expected 2 in queue, got %d", len(s.queue))
	}

	s.removeFromQueue("alice")
	if len(s.queue) != 1 {
		t.Fatalf("expected 1 in queue after removal, got %d", len(s.queue))
	}
	if s.queue[0].Name != "bob" {
		t.Errorf("expected 'bob' remaining, got %q", s.queue[0].Name)
	}

	s.removeFromQueue("nonexistent") // should not panic
}
