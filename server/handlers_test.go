package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setupJoinTest(t *testing.T) (*Server, func()) {
	qoBin := createTempFile(t, "qo")
	archive := createTempFile(t, "archive")

	cfg := &Config{
		EventCode:     "test-event",
		AdminSecret:   "admin-secret-123",
		QoBinaryPath:  qoBin,
		ArchivePath:   archive,
		Password:      "test-pass",
		Key:           "test-key",
		MaxConcurrent: 2,
		QoDuration:    "90m",
		Port:          8080,
	}
	s := NewServer(cfg)
	return s, func() {}
}

func TestJoin_Valid(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	body := strings.NewReader("name=student42&code=test-event")
	req := httptest.NewRequest("POST", "/join", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	s.handleJoin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp joinResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("expected status success, got %s", resp.Status)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestJoin_InvalidEventCode(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	body := strings.NewReader("name=student42&code=wrong-code")
	req := httptest.NewRequest("POST", "/join", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	s.handleJoin(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp joinResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "error" || !strings.Contains(resp.Message, "event code") {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestJoin_MissingName(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	body := strings.NewReader("code=test-event")
	req := httptest.NewRequest("POST", "/join", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	s.handleJoin(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestJoin_CapacityFull(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	for i := 0; i < s.config.MaxConcurrent; i++ {
		body := strings.NewReader("name=student" + itoa(i) + "&code=test-event")
		req := httptest.NewRequest("POST", "/join", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		s.handleJoin(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("failed to fill cap at iteration %d: %d", i, w.Code)
		}
	}

	body := strings.NewReader("name=overflow&code=test-event")
	req := httptest.NewRequest("POST", "/join", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleJoin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for queued response, got %d", w.Code)
	}

	var resp joinResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "queued" {
		t.Errorf("expected queued status, got %s", resp.Status)
	}
	if resp.Position <= 0 {
		t.Errorf("expected positive queue position, got %d", resp.Position)
	}
}

func TestJoin_ReconnectExisting(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	body := strings.NewReader("name=reconnect_user&code=test-event")
	req := httptest.NewRequest("POST", "/join", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleJoin(w, req)

	var first joinResponse
	json.Unmarshal(w.Body.Bytes(), &first)
	if first.Token == "" {
		t.Fatal("expected token on first join")
	}

	body2 := strings.NewReader("name=reconnect_user&code=test-event")
	req2 := httptest.NewRequest("POST", "/join", body2)
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	s.handleJoin(w2, req2)

	var second joinResponse
	json.Unmarshal(w2.Body.Bytes(), &second)
	if second.Token != first.Token {
		t.Errorf("expected same token on reconnect, got %s vs %s", second.Token, first.Token)
	}
}

func TestJoin_RateLimited(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	s.limiter = NewRateLimiter(2, 999999) // very long window so we stay rate-limited

	body := strings.NewReader("name=s1&code=test-event")
	req := httptest.NewRequest("POST", "/join", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "192.168.1.1:12345"

	w := httptest.NewRecorder()
	s.handleJoin(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first request should succeed: %d", w.Code)
	}

	body2 := strings.NewReader("name=s2&code=test-event")
	req2 := httptest.NewRequest("POST", "/join", body2)
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.RemoteAddr = "192.168.1.1:12345"

	w2 := httptest.NewRecorder()
	s.handleJoin(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second request should succeed: %d", w2.Code)
	}

	body3 := strings.NewReader("name=s3&code=test-event")
	req3 := httptest.NewRequest("POST", "/join", body3)
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req3.RemoteAddr = "192.168.1.1:12345"

	w3 := httptest.NewRecorder()
	s.handleJoin(w3, req3)
	if w3.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for rate limited request, got %d", w3.Code)
	}
}

func TestAdminAuth_NoHeader(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/admin/state", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "unauthorized" {
		t.Errorf("expected error=unauthorized, got %v", body)
	}
}

func TestAdminAuth_WrongHeader(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/admin/state", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	req.Header.Set("X-Admin-Token", "wrong-secret")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "unauthorized" {
		t.Errorf("expected error=unauthorized, got %v", body)
	}
}

func TestAdminAuth_CorrectHeader(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/admin/state", nil)
	req.Header.Set("X-Admin-Token", "admin-secret-123")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp adminStateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.MaxConcurrent != 2 {
		t.Errorf("expected maxConcurrent 2, got %d", resp.MaxConcurrent)
	}
}

func TestAdminAuth_RateLimited(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	s.adminLimiter = NewRateLimiter(3, 999999) // long window = stays rate-limited

	ip := "10.0.0.99:12345"

	// 3 failed attempts to hit the rate limit
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/admin/state", nil)
		req.RemoteAddr = ip
		req.Header.Set("X-Admin-Token", "wrong")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}

	// 4th wrong attempt should be rate limited (only failed attempts count)
	req := httptest.NewRequest("GET", "/admin/state", nil)
	req.RemoteAddr = ip
	req.Header.Set("X-Admin-Token", "still-wrong")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 on 4th failed attempt, got %d", w.Code)
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "unauthorized" {
		t.Errorf("expected error=unauthorized, got %v", body)
	}
}

func TestAdminAuth_DifferentIPNotAffected(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	s.adminLimiter = NewRateLimiter(1, 999999)

	// Lock out IP 10.0.0.1
	req := httptest.NewRequest("GET", "/admin/state", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Admin-Token", "wrong")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	// Different IP should still be able to auth correctly
	req2 := httptest.NewRequest("GET", "/admin/state", nil)
	req2.Header.Set("X-Admin-Token", "admin-secret-123")
	w2 := httptest.NewRecorder()
	s.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 for different IP, got %d", w2.Code)
	}
}

func TestAdminKill_RequiresAuth(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/admin/kill?token=abc", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminShutdown_RequiresAuth(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/admin/shutdown", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminState_RequiresAuth(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/admin/state", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminHTML_ServesWithoutAuth(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/admin", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (login page HTML, auth via JS), got %d", w.Code)
	}
}

func TestAdminHTML_WithAuth(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("X-Admin-Token", "admin-secret-123")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestJoin_ShuttingDown(t *testing.T) {
	s, cleanup := setupJoinTest(t)
	defer cleanup()
	s.shutdown = true

	body := strings.NewReader("name=student42&code=test-event")
	req := httptest.NewRequest("POST", "/join", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	s.handleJoin(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when shutting down, got %d", w.Code)
	}
}
