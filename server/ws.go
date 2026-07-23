package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
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

		// Process is dead — clean up old Term so we fall through
		// to spawn a fresh qo below.
		log.Printf("session %s process is dead, reinitializing", token)
		session.Term.Close()
		session.Term = nil
		session.Cmd = nil
	}
	session.mu.Unlock()

	cmd := exec.Command(s.config.QoBinaryPath, "start",
		"-i", session.StudentID,
		"-a", s.config.ArchivePath,
		"-p", s.config.Password,
		"-k", s.config.Key,
		"-d", s.config.QoDuration,
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

	termFile := master

	session.mu.Lock()
	session.Cmd = cmd
	session.Term = termFile
	session.SetState(SessionActive)
	session.mu.Unlock()

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade (new %s): %v", token, err)
		s.cleanupSession(token)
		return
	}

	pipeTermToWS(s, conn, termFile, session)
}

func pipeTermToWS(srv *Server, conn *websocket.Conn, term *os.File, session *Session) {
	token := session.Token
	done := make(chan struct{}, 2)

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
				if err := conn.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
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
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
			conn.SetWriteDeadline(time.Time{})
		}
	}()

	go func() {
		check := time.NewTicker(5 * time.Second)
		defer check.Stop()
		for range check.C {
			session.mu.Lock()
			st := session.State()
			session.mu.Unlock()
			if st != SessionActive {
				continue
			}
			idle := time.Since(session.LastActive())
			if idle < srv.config.IdleTimeout {
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

	<-done
	<-done

	conn.Close()

	session.mu.Lock()
	if session.State() == SessionActive {
		log.Printf("session %s disconnected, entering orphaned state", token)
		session.SetState(SessionOrphaned)
	}

	// Check if the underlying process is still alive. If the PTY slave
	// was closed (qo exited), the master will return EIO on next read.
	// We close the Term so the next reconnect spawns a fresh qo.
	if session.Cmd != nil && session.Cmd.Process != nil {
		if err := session.Cmd.Process.Signal(syscall.Signal(0)); err != nil {
			log.Printf("session %s process exited, cleaning up PTY", token)
			if session.Term != nil {
				session.Term.Close()
				session.Term = nil
			}
			session.Cmd = nil
		}
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

	session.Close()
	s.manager.RemoveSession(token)
	log.Printf("session %s cleaned up", token)
}

func wsNotify(session *Session, msg wsMessage) {
	if session.Term != nil {
		data, _ := json.Marshal(msg)
		session.Term.Write(append(data, '\n'))
	}
}
