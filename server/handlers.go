package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mohammed-niri/qo-learn-tool/server/webassets"
)

type Server struct {
	config       *Config
	manager      *SessionManager
	limiter      *RateLimiter
	adminLimiter *RateLimiter
	shutdown     bool
	startTime    time.Time
	queue        []queueEntry
	queueMu      sync.Mutex
	metaTitle    string
	metaDifficulty string
}

func NewServer(cfg *Config) *Server {
	s := &Server{
		config:       cfg,
		manager:      NewSessionManager(cfg.MaxConcurrent),
		limiter:      NewRateLimiter(5, cfg.GracePeriod),
		adminLimiter: NewRateLimiter(5, 1*time.Minute),
		startTime:    time.Now(),
		metaTitle:    "Untitled",
		metaDifficulty: "unknown",
	}
	s.loadChallengeMeta()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/static/") {
		staticFS, err := fs.Sub(webassets.FS, "static")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))).ServeHTTP(w, r)
		return
	}

	// Admin routes
	if r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, "/admin/") {
		switch r.Method + " " + r.URL.Path {
		case "GET /admin":
			// Serve the login page HTML without auth (login form sends token via JS)
			s.handleAdmin(w, r)
		case "GET /admin/state":
			if s.adminAuth(w, r) { s.handleAdminState(w, r) }
		case "POST /admin/kill":
			if s.adminAuth(w, r) { s.handleAdminKill(w, r) }
		case "POST /admin/shutdown":
			if s.adminAuth(w, r) { s.handleAdminShutdown(w, r) }
		case "GET /admin/leaderboard":
			if s.adminAuth(w, r) { s.handleLeaderboardPage(w, r) }
		default:
			http.NotFound(w, r)
		}
		return
	}

	switch r.Method + " " + r.URL.Path {
	case "GET /":
		s.handleLogin(w, r)
	case "POST /join":
		s.handleJoin(w, r)
	case "GET /terminal":
		s.handleTerminal(w, r)
	case "GET /ws":
		s.handleWebSocket(w, r)
	case "GET /api/leaderboard":
		s.handleLeaderboardData(w, r)
	case "POST /api/solved":
		s.handleSolved(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.shutdown {
		http.Error(w, "Server shutting down", http.StatusServiceUnavailable)
		return
	}
	serveEmbedded(w, r, "index.html")
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "token required", http.StatusBadRequest)
		return
	}

	s.manager.mu.RLock()
	_, ok := s.manager.sessions[token]
	s.manager.mu.RUnlock()
	if !ok {
		http.Error(w, "invalid token", http.StatusNotFound)
		return
	}

	serveEmbedded(w, r, "terminal.html")
}

type joinResponse struct {
	Status     string `json:"status"`
	Token      string `json:"token,omitempty"`
	Position   int    `json:"position,omitempty"`
	Message    string `json:"message"`
	Title      string `json:"title,omitempty"`
	Difficulty string `json:"difficulty,omitempty"`
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	if s.shutdown {
		writeJSON(w, http.StatusServiceUnavailable, joinResponse{
			Status:  "error",
			Message: "Event is shutting down, no new sessions accepted",
		})
		return
	}

	ip := clientIP(r)
	if !s.limiter.Allow(ip) {
		writeJSON(w, http.StatusTooManyRequests, joinResponse{
			Status:  "error",
			Message: "Too many attempts. Please wait before retrying",
		})
		return
	}

	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, joinResponse{
			Status:  "error",
			Message: "Invalid form data",
		})
		return
	}

	code := r.FormValue("code")
	if code != s.config.EventCode {
		writeJSON(w, http.StatusBadRequest, joinResponse{
			Status:  "error",
			Message: "Invalid event code",
		})
		return
	}

	studentID := r.FormValue("name")
	if studentID == "" {
		studentID = r.FormValue("id")
	}
	if studentID == "" {
		writeJSON(w, http.StatusBadRequest, joinResponse{
			Status:  "error",
			Message: "Student name is required",
		})
		return
	}

	session, err := s.manager.NewSession(studentID)
	if err != nil {
		if strings.Contains(err.Error(), "concurrency cap reached") {
			s.addToQueue(studentID)
			var queuePos int
			s.queueMu.Lock()
			for i, e := range s.queue {
				if e.Name == studentID {
					queuePos = i + 1
					break
				}
			}
			s.queueMu.Unlock()
			if queuePos < 1 {
				queuePos = 1
			}
			writeJSON(w, http.StatusOK, joinResponse{
				Status:   "queued",
				Position: queuePos,
				Message:  "Event is full. You are #" + itoa(queuePos) + " in queue",
			})
			return
		}
		if strings.Contains(err.Error(), "already has an active session") {
			s.removeFromQueue(studentID)
			tok, _ := s.manager.LookupByStudentID(studentID)
			existing, _ := s.manager.GetSession(tok)
			title := s.metaTitle
			difficulty := s.metaDifficulty
			if existing != nil {
				title = existing.Title
				difficulty = existing.Difficulty
			}
			writeJSON(w, http.StatusOK, joinResponse{
				Status:     "success",
				Token:      tok,
				Title:      title,
				Difficulty: difficulty,
				Message:    "Reconnecting to existing session",
			})
			return
		}
		log.Printf("session creation error: %v", err)
		writeJSON(w, http.StatusInternalServerError, joinResponse{
			Status:  "error",
			Message: "Internal error",
		})
		return
	}

	session.Title = s.metaTitle
	session.Difficulty = s.metaDifficulty

	s.removeFromQueue(studentID)

	writeJSON(w, http.StatusOK, joinResponse{
		Status:     "success",
		Token:      session.Token,
		Title:      session.Title,
		Difficulty: session.Difficulty,
		Message:    "Session created. Connecting...",
	})
}

func (s *Server) handleSolved(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token required"})
		return
	}

	session, ok := s.manager.GetSession(token)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid token"})
		return
	}

	n := session.IncrementScore()
	log.Printf("session %s solved challenge, score now %d", token, n)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "score": n})
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if parts := strings.Split(fwd, ","); len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func serveEmbedded(w http.ResponseWriter, r *http.Request, name string) {
	data, err := webassets.FS.ReadFile(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

func (s *Server) loadChallengeMeta() {
	out, err := exec.Command(s.config.QoBinaryPath, "meta",
		"-a", s.config.ArchivePath,
		"-p", s.config.Password,
		"-k", s.config.Key,
	).Output()
	if err != nil {
		log.Printf("warning: failed to load challenge metadata: %v (using defaults)", err)
		return
	}

	var meta struct {
		Title      string `json:"title"`
		Difficulty string `json:"difficulty"`
	}
	if err := json.Unmarshal(out, &meta); err != nil {
		log.Printf("warning: failed to parse challenge metadata: %v (using defaults)", err)
		return
	}

	if meta.Title != "" {
		s.metaTitle = meta.Title
	}
	if meta.Difficulty != "" {
		s.metaDifficulty = meta.Difficulty
	}
	log.Printf("loaded challenge metadata: title=%q difficulty=%q", s.metaTitle, s.metaDifficulty)
}

func (s *Server) adminAuth(w http.ResponseWriter, r *http.Request) bool {
	ip := clientIP(r)
	token := r.Header.Get("X-Admin-Token")

	if subtle.ConstantTimeCompare([]byte(token), []byte(s.config.AdminSecret)) != 1 {
		if !s.adminLimiter.Allow(ip) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "unauthorized"})
			return false
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}

	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
