package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLeaderboardData_Empty(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/leaderboard", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var entries []leaderboardEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if entries == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(entries) != 0 {
		t.Errorf("expected empty leaderboard, got %d entries", len(entries))
	}
}

func TestLeaderboardData_WithScores(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	// Override capacity to allow 3 sessions (default is 2).
	s.config.MaxConcurrent = 3
	s.manager = NewSessionManager(3)

	s1, _ := s.manager.NewSession("alice", "")
	s2, _ := s.manager.NewSession("bob", "")
	s3, _ := s.manager.NewSession("carol", "")

	s1.SetScore(5)
	s2.SetScore(3)
	s3.SetScore(7)

	req := httptest.NewRequest("GET", "/api/leaderboard", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var entries []leaderboardEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Order not guaranteed (map iteration), but all should be present.
	byName := make(map[string]int)
	for _, e := range entries {
		byName[e.Name] = e.Solved
	}
	if byName["alice"] != 5 {
		t.Errorf("expected alice=5, got %d", byName["alice"])
	}
	if byName["bob"] != 3 {
		t.Errorf("expected bob=3, got %d", byName["bob"])
	}
	if byName["carol"] != 7 {
		t.Errorf("expected carol=7, got %d", byName["carol"])
	}
	if entries[0].Avatar == "" {
		t.Error("expected non-empty avatar")
	}
}
