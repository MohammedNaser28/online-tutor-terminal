package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type wsMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "token required", http.StatusBadRequest)
		return
	}

	session, ok := s.manager.GetSession(token)
	if !ok {
		http.Error(w, "invalid token", http.StatusBadRequest)
		return
	}

	session.mu.Lock()
	state := session.State()

	if state == SessionClosed {
		session.mu.Unlock()
		http.Error(w, "session expired", http.StatusBadRequest)
		return
	}

	// If there's an existing PTY (from a previous connection), reuse it.
	// Check if the underlying process is still alive first.
	if session.Term != nil {
		dead := false
		if session.Cmd != nil && session.Cmd.Process != nil {
			if err := session.Cmd.Process.Signal(syscall.Signal(0)); err != nil {
				dead = true
			}
		}

		if !dead {
			term := session.Term
			session.SetState(SessionActive)
			session.mu.Unlock()

			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Printf("ws upgrade (reuse %s): %v", token, err)
				return
			}

			pipeTermToWS(s, conn, term, session)
			return
		}

		log.Printf("session %s process is dead, reinitializing", token)
		if session.Term != nil {
			session.Term.Close()
			session.Term = nil
		}
		session.Cmd = nil
	}

	// If the session has an existing rootfs that's still on disk, restart
	// qo-init inside it (preserves progress and ChallengeState).
	rootfsExists := session.RootfsPath != ""
	if rootfsExists {
		if _, err := os.Stat(session.RootfsPath); err != nil {
			rootfsExists = false
		}
	}

	if rootfsExists {
		session.mu.Unlock()

		master, slave, err := pty.Open()
		if err != nil {
			log.Printf("pty open for %s: %v", token, err)
			http.Error(w, "failed to start session", http.StatusInternalServerError)
			return
		}
		defer slave.Close()

		if err := pty.Setsize(master, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
			master.Close()
			log.Printf("pty setsize for %s: %v", token, err)
			http.Error(w, "failed to start session", http.StatusInternalServerError)
			return
		}

		qoInit := findQoInit(s.config.QoBinaryPath)
		if qoInit == "" {
			master.Close()
			log.Printf("qo-init not found for %s", token)
			http.Error(w, "qo-init not found", http.StatusInternalServerError)
			return
		}

		cmd := exec.Command(qoInit, session.RootfsPath)
		cmd.Stdin = slave
		cmd.Stdout = slave
		cmd.Stderr = slave
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
		}

		if err := cmd.Start(); err != nil {
			master.Close()
			log.Printf("qo-init spawn for %s: %v", token, err)
			http.Error(w, "failed to start session", http.StatusInternalServerError)
			return
		}

		session.mu.Lock()
		session.Cmd = cmd
		session.Term = master
		session.SetState(SessionActive)
		session.mu.Unlock()

		// Don't restart pollChallengeRequests or discoverAndInitChallenge —
		// they're still running from the first connection with the correct
		// RootfsPath and ChallengeState.

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws upgrade (reconnect %s): %v", token, err)
			s.cleanupSession(token)
			return
		}

		log.Printf("session %s reconnected, reusing rootfs %s", token, session.RootfsPath)
		pipeTermToWS(s, conn, master, session)
		return
	}

	session.mu.Unlock()

	// Generate a unique session path for this user. This avoids the race
	// where discoverAndInitChallenge picks another user's rootfs directory.
	suffix := fmt.Sprintf("%04x", rand.Int31())
	sessionRootfs := filepath.Join("/tmp/qo-sessions", fmt.Sprintf("%s-%s", session.StudentID, suffix))

	session.mu.Lock()
	session.RootfsPath = sessionRootfs
	session.mu.Unlock()

	// check.sh stays inside the sandbox: `go` executes it locally and
	// reports completion via the "solved" IPC action (see challenge.go).
	// Validators-only levels fall back to the server-side "go" IPC action.

	cmd := exec.Command(s.config.QoBinaryPath, "start",
		"-i", session.StudentID,
		"-a", s.config.ArchivePath,
		"-p", s.config.Password,
		"-k", s.config.Key,
		"-d", s.config.QoDuration,
	)

	cmd.Env = append(os.Environ(),
		"QO_STUDENT_NAME="+session.StudentID,
		"QO_SESSION_PATH="+sessionRootfs,
	)

	master, slave, err := pty.Open()
	if err != nil {
		log.Printf("pty open for %s: %v", token, err)
		http.Error(w, "failed to start session", http.StatusInternalServerError)
		return
	}
	defer slave.Close()

	if err := pty.Setsize(master, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		master.Close()
		log.Printf("pty setsize for %s: %v", token, err)
		http.Error(w, "failed to start session", http.StatusInternalServerError)
		return
	}

	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setctty: true,
		Setsid:  true,
		Ctty:    0,
	}

	if err := cmd.Start(); err != nil {
		master.Close()
		log.Printf("pty spawn for %s: %v", token, err)
		http.Error(w, "failed to start session", http.StatusInternalServerError)
		return
	}

	session.mu.Lock()
	session.Cmd = cmd
	session.SetState(SessionActive)
	session.mu.Unlock()

	// Load challenge metadata synchronously so pollChallengeRequests
	// always sees a ready Challenge state. This runs `qo meta` (~1s).
	if s.config.ArchivePath != "" {
		meta, err := LoadChallengeMeta(s.config.QoBinaryPath, s.config.ArchivePath, s.config.Password, s.config.Key)
		if err != nil {
			log.Printf("load meta for %s: %v", token, err)
		} else {
			session.mu.Lock()
			if len(meta.Levels) > 0 {
				session.Title = meta.Title
				session.Difficulty = meta.Difficulty
				session.Challenge = NewChallengeState(meta.Levels)
				log.Printf("session %s loaded challenge meta: title=%q levels=%d", token, meta.Title, len(meta.Levels))
			} else {
				log.Printf("session %s loaded challenge meta but levels empty, trying single question", token)
				if meta.Question != "" {
					meta.Levels = []ChallengeLevel{{
						ID:       1,
						Title:    meta.Title,
						Question: meta.Question,
						Hint:     meta.DefaultHint,
					}}
					session.Title = meta.Title
					session.Difficulty = meta.Difficulty
					session.Challenge = NewChallengeState(meta.Levels)
					log.Printf("session %s created single level from question", token)
				} else {
					log.Printf("session %s meta has no levels and no question, skipping", token)
				}
			}
			session.mu.Unlock()
		}
	}

	// Read check.sh content from extracted rootfs and delete from sandbox.
	if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
		loadCheckScripts(session.RootfsPath, session.Challenge.Levels)

		// Point the in-sandbox `go` helper at the first level so local
		// check.sh execution works before any quest/level command.
		levelFile := filepath.Join(session.RootfsPath, "rootfs", "tmp", ".qo-current-level")
		_ = os.WriteFile(levelFile, []byte(fmt.Sprintf("level%d", session.Challenge.Levels[0].ID)), 0644)
	}

	go s.pollChallengeRequests(session)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade (new %s): %v", token, err)
		s.cleanupSession(token)
		return
	}

	pipeTermToWS(s, conn, master, session)
}

func pipeTermToWS(srv *Server, conn *websocket.Conn, term *os.File, session *Session) {
	token := session.Token
	done := make(chan struct{}, 2)
	var connMu sync.Mutex

	const (
		pingPeriod = 10 * time.Second
		pongWait   = 30 * time.Second
	)

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 4096)
		for {
			n, err := term.Read(buf)
			if n > 0 {
				connMu.Lock()
				err := conn.WriteMessage(websocket.TextMessage, buf[:n])
				connMu.Unlock()
				if err != nil {
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("pty read %s: %v", token, err)
				}
				return
			}
		}
	}()

	go func() {
		defer func() { done <- struct{}{} }()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					log.Printf("ws read %s: %v", token, err)
				}
				return
			}
			conn.SetReadDeadline(time.Now().Add(pongWait))
			session.Touch()
			var resize struct {
				Type string `json:"type"`
				Cols int    `json:"cols"`
				Rows int    `json:"rows"`
			}
			if json.Unmarshal(data, &resize) == nil && resize.Type == "resize" && resize.Cols > 0 && resize.Rows > 0 {
				if err := pty.Setsize(term, &pty.Winsize{Rows: uint16(resize.Rows), Cols: uint16(resize.Cols)}); err != nil {
					log.Printf("pty resize %s: %v", token, err)
				}
				continue
			}
			if _, err := term.Write(data); err != nil {
				log.Printf("pty write %s: %v", token, err)
				return
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for range ticker.C {
			connMu.Lock()
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			err := conn.WriteMessage(websocket.PingMessage, nil)
			conn.SetWriteDeadline(time.Time{})
			connMu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	go func() {
		check := time.NewTicker(5 * time.Second)
		defer check.Stop()
		warned := false
		for range check.C {
			session.mu.Lock()
			st := session.State()
			session.mu.Unlock()
			if st != SessionActive {
				continue
			}
			idle := time.Since(session.LastActive())
			remaining := srv.config.IdleTimeout - idle
			if remaining > time.Minute {
				warned = false
				continue
			}
			if !warned && remaining > 0 {
				// Warn shortly before the timeout so the user can act.
				warned = true
				wsNotify(session, wsMessage{
					Type:    "warn",
					Message: fmt.Sprintf("⚠️ You will be disconnected in %d second(s) due to inactivity — press any key to stay connected.", int(remaining.Seconds())),
				})
				continue
			}
			if remaining > 0 {
				continue
			}
			log.Printf("session %s idle for %v, closing", token, idle)
			wsNotify(session, wsMessage{Type: "shutdown", Message: "Session closed due to inactivity"})
			time.Sleep(3 * time.Second)
			conn.Close()
			srv.cleanupSession(token)
			return
		}
	}()

	// Wait for one goroutine to finish (usually the WebSocket reader on disconnect).
	// Then close the PTY master to unblock the PTY reader, so pipeTermToWS can exit.
	<-done

	// Close the PTY master to unblock the PTY reader goroutine.
	// This is safe: the process inside the sandbox (qo-init/bash) still has the slave
	// end open, but the master being closed means they'll get EIO on next read/write
	// and eventually exit. We accept this — the term is replaced on reconnect.
	term.Close()

	// Now wait for the PTY reader to finish
	<-done

	conn.Close()

	session.mu.Lock()
	session.Term = nil
	session.Cmd = nil

	if session.State() == SessionActive {
		log.Printf("session %s disconnected, entering orphaned state", token)
		session.SetState(SessionOrphaned)
	}
	session.mu.Unlock()

	go func() {
		time.Sleep(srv.config.GracePeriod)
		session.mu.Lock()
		if session.State() == SessionOrphaned {
			log.Printf("session %s grace period expired, closing", token)
			srv.cleanupSession(token)
		}
		session.mu.Unlock()
	}()
}

func (s *Server) cleanupSession(token string) {
	session, ok := s.manager.GetSession(token)
	if !ok {
		return
	}

	rootfs := session.RootfsPath
	session.Close()
	s.manager.RemoveSession(token)

	if rootfs != "" {
		if err := os.RemoveAll(rootfs); err != nil {
			log.Printf("cleanup: remove rootfs %s: %v", rootfs, err)
		}
	}
	log.Printf("session %s cleaned up", token)
}

// findQoInit locates the qo-init binary using the same search order as
// the sandbox package's findHelper().
func findQoInit(qoBinaryPath string) string {
	candidates := []string{
		filepath.Join(filepath.Dir(qoBinaryPath), "qo-init"),
		"/usr/local/bin/qo-init",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func (s *Server) discoverAndInitChallenge(session *Session) {
	sessionsDir := "/tmp/qo-sessions"
	studentID := session.StudentID
	prefix := studentID + "-"

	for i := 0; i < 200; i++ {
		entries, err := os.ReadDir(sessionsDir)
		if err == nil {
			var bestPath string
			var bestTime time.Time
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				if !strings.HasPrefix(e.Name(), prefix) {
					continue
				}
				info, err := e.Info()
				if err != nil {
					continue
				}
				mt := info.ModTime()
				if bestPath == "" || mt.After(bestTime) {
					bestPath = filepath.Join(sessionsDir, e.Name())
					bestTime = mt
				}
			}
			if bestPath != "" {
				session.mu.Lock()
				session.RootfsPath = bestPath
				session.mu.Unlock()

				if s.config.ArchivePath == "" {
					return
				}
				meta, err := LoadChallengeMeta(s.config.QoBinaryPath, s.config.ArchivePath, s.config.Password, s.config.Key)
				if err != nil {
					log.Printf("discover: LoadChallengeMeta: %v", err)
					return
				}
				session.mu.Lock()
				if len(meta.Levels) > 0 {
					session.Title = meta.Title
					session.Difficulty = meta.Difficulty
					session.Challenge = NewChallengeState(meta.Levels)
				} else {
					levels, discErr := DiscoverLevelsFromRootfs(bestPath)
					if discErr == nil && len(levels) > 0 {
						session.Challenge = NewChallengeState(levels)
					} else {
						log.Printf("discover: no levels found via meta or rootfs for %s", session.Token)
					}
				}
				session.mu.Unlock()
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Printf("warning: could not discover session rootfs for %s", session.Token)
}

func wsNotify(session *Session, msg wsMessage) {
	if session.Term != nil {
		data, _ := json.Marshal(msg)
		session.Term.Write(append(data, '\n'))
	}
}
