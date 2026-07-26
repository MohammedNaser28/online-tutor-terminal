package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindQoInit_SameDir(t *testing.T) {
	tmpDir := t.TempDir()
	qoBin := filepath.Join(tmpDir, "qo")
	initBin := filepath.Join(tmpDir, "qo-init")
	if err := os.WriteFile(qoBin, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(initBin, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	path := findQoInit(qoBin)
	if path != initBin {
		t.Errorf("expected %q, got %q", initBin, path)
	}
}

func TestCleanupSession_RemovesRootfs(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	session, _ := s.manager.NewSession("test-student")
	rootfs, err := os.MkdirTemp("", "qo-test-cleanup-*")
	if err != nil {
		t.Fatal(err)
	}
	session.RootfsPath = rootfs

	s.cleanupSession(session.Token)

	if _, err := os.Stat(rootfs); !os.IsNotExist(err) {
		t.Error("rootfs should be removed after cleanup")
	}

	_, ok := s.manager.GetSession(session.Token)
	if ok {
		t.Error("session should be removed after cleanup")
	}
}

func TestCleanupSession_NoRootfs(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	session, _ := s.manager.NewSession("test-student")
	// RootfsPath is empty — should not crash.

	s.cleanupSession(session.Token)

	_, ok := s.manager.GetSession(session.Token)
	if ok {
		t.Error("session should be removed")
	}
}

func TestCleanupSession_NonexistentToken(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	s.cleanupSession("nonexistent") // should not panic
}

func TestWsNotify_NoTerm(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	session, _ := s.manager.NewSession("test-student")

	// Should not panic when Term is nil.
	wsNotify(session, wsMessage{Type: "shutdown", Message: "bye"})
}

func Test_itoa(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{999, "999"},
		{-1, "-1"},
		{-42, "-42"},
		{123456789, "123456789"},
	}
	for _, c := range cases {
		got := itoa(c.n)
		if got != c.want {
			t.Errorf("itoa(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1, 10.0.0.1")
	req.RemoteAddr = "192.168.1.1:12345"

	ip := clientIP(req)
	if ip != "203.0.113.1" {
		t.Errorf("expected 203.0.113.1, got %q", ip)
	}
}

func TestClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.42:8080"

	ip := clientIP(req)
	if ip != "10.0.0.42" {
		t.Errorf("expected 10.0.0.42, got %q", ip)
	}
}

// ─── Challenge API handlers ─────────────────────────────────────────────────

func setupChallengeAPI(t *testing.T, levels []ChallengeLevel) (*Server, string, func()) {
	t.Helper()
	s, cleanup := setupJoinTest(t)

	session, err := s.manager.NewSession("challenge-user")
	if err != nil {
		cleanup()
		t.Fatalf("NewSession: %v", err)
	}

	rootfs, rCleanup2 := createRootfs(t, []int{1})
	session.mu.Lock()
	session.RootfsPath = rootfs
	session.Title = "Test Challenge"
	if len(levels) > 0 {
		session.Challenge = NewChallengeState(levels)
	}
	session.mu.Unlock()

	return s, session.Token, func() {
		rCleanup2()
		cleanup()
	}
}

func TestHandleChallengeQuest(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One", Question: "Find X", Hint: "Look here"},
		{ID: 2, Title: "Two", Question: "Find Y"},
	}
	s, token, cleanup := setupChallengeAPI(t, levels)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/challenge/quest?token="+token, nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["question"] != "Find X" {
		t.Errorf("expected 'Find X', got %v", resp["question"])
	}
	if resp["level"] != float64(1) {
		t.Errorf("expected level 1, got %v", resp["level"])
	}
}

func TestHandleChallengeHint(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One", Question: "Q", Hint: "Look closer"},
	}
	s, token, cleanup := setupChallengeAPI(t, levels)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/challenge/hint?token="+token, nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["hint"] != "Look closer" {
		t.Errorf("expected 'Look closer', got %q", resp["hint"])
	}
}

func TestHandleChallengeHint_Empty(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One", Question: "Q"},
	}
	s, token, cleanup := setupChallengeAPI(t, levels)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/challenge/hint?token="+token, nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["hint"] != "No hint available." {
		t.Errorf("expected 'No hint available.', got %q", resp["hint"])
	}
}

func TestHandleChallengeMap(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One", Question: "Q"},
		{ID: 2, Title: "Two", Question: "Q2"},
	}
	s, token, cleanup := setupChallengeAPI(t, levels)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/challenge/map?token="+token, nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total_levels"] != float64(2) {
		t.Errorf("expected total_levels 2, got %v", resp["total_levels"])
	}
}

func TestHandleChallengeStatus(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One", Question: "Q"},
	}
	s, token, cleanup := setupChallengeAPI(t, levels)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/challenge/status?token="+token, nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["level"] != float64(1) {
		t.Errorf("expected level 1, got %v", resp["level"])
	}
	if resp["completed"] != false {
		t.Error("expected not completed")
	}
}

func TestHandleChallengeGo_CheckScript(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One", Question: "Q", CheckScript: "exit 0"},
	}
	s, token, cleanup := setupChallengeAPI(t, levels)
	defer cleanup()

	req := httptest.NewRequest("POST", "/api/challenge/go?token="+token, nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["passed"] != true {
		t.Errorf("expected passed=true for exit 0, got %v", resp["passed"])
	}
}

func TestHandleChallengeGo_Fails(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One", Question: "Q", CheckScript: "exit 1"},
	}
	s, token, cleanup := setupChallengeAPI(t, levels)
	defer cleanup()

	req := httptest.NewRequest("POST", "/api/challenge/go?token="+token, nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["passed"] != false {
		t.Errorf("expected passed=false for exit 1, got %v", resp["passed"])
	}
}

func TestChallengeAPI_InvalidToken(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/challenge/quest?token=badtoken", ""},
		{"GET", "/api/challenge/hint?token=badtoken", ""},
		{"GET", "/api/challenge/map?token=badtoken", ""},
		{"GET", "/api/challenge/status?token=badtoken", ""},
		{"POST", "/api/challenge/go", "token=badtoken"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var req *http.Request
			if ep.method == "POST" {
				body := strings.NewReader(ep.body)
				req = httptest.NewRequest(ep.method, ep.path, body)
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			} else {
				req = httptest.NewRequest(ep.method, ep.path, nil)
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Errorf("expected 404 for bad token, got %d", w.Code)
			}
		})
	}
}

func TestChallengeAPI_NoChallenge(t *testing.T) {
	// Token valid but no ChallengeState set.
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	session, _ := s.manager.NewSession("no-challenge-user")
	token := session.Token

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/challenge/quest?token=" + token},
		{"GET", "/api/challenge/hint?token=" + token},
		{"GET", "/api/challenge/map?token=" + token},
		{"GET", "/api/challenge/status?token=" + token},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", w.Code)
			}
		})
	}
}
