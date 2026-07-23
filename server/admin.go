package main

import (
	"log"
	"net/http"
	"time"
)

type queueEntry struct {
	Name   string `json:"name"`
	Joined int64  `json:"joined"` // unix millis
}

type adminStateResponse struct {
	Sessions      []sessionInfo `json:"sessions"`
	Queue         []queueEntry  `json:"queue"`
	StartTime     int64         `json:"startTime"`
	ShuttingDown  bool          `json:"shuttingDown"`
	MaxConcurrent int           `json:"maxConcurrent"`
}

type sessionInfo struct {
	Token   string `json:"token"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Created int64  `json:"created"`
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	serveEmbedded(w, r, "admin.html")
}

func (s *Server) handleAdminState(w http.ResponseWriter, r *http.Request) {
	var sessions []sessionInfo
	s.manager.AllSessions(func(ses *Session) {
		ses.mu.Lock()
		st := ses.State().String()
		ses.mu.Unlock()
		sessions = append(sessions, sessionInfo{
			Token:   ses.Token,
			Name:    ses.StudentID,
			State:   st,
			Created: ses.CreatedAt.UnixMilli(),
		})
	})

	s.queueMu.Lock()
	queue := make([]queueEntry, len(s.queue))
	copy(queue, s.queue)
	s.queueMu.Unlock()

	writeJSON(w, http.StatusOK, adminStateResponse{
		Sessions:      sessions,
		Queue:         queue,
		StartTime:     s.startTime.UnixMilli(),
		ShuttingDown:  s.shutdown,
		MaxConcurrent: s.config.MaxConcurrent,
	})
}

func (s *Server) handleAdminKill(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token required"})
		return
	}

	session, ok := s.manager.GetSession(token)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}

	name := session.StudentID
	session.Close()
	s.manager.RemoveSession(token)
	log.Printf("admin killed session %s (%s)", token, name)

	writeJSON(w, http.StatusOK, map[string]string{"status": "killed"})
}

func (s *Server) handleAdminShutdown(w http.ResponseWriter, r *http.Request) {
	s.shutdown = true
	log.Print("admin initiated event shutdown")

	s.manager.AllSessions(func(session *Session) {
		session.mu.Lock()
		if session.Term != nil {
			wsNotify(session, wsMessage{
				Type:    "shutdown",
				Message: "Event is ending. This session will close in 30 seconds.",
			})
		}
		session.mu.Unlock()
	})

	go func() {
		time.Sleep(30 * time.Second)
		s.manager.AllSessions(func(session *Session) {
			session.Close()
		})
		log.Print("all sessions terminated after admin shutdown")
	}()

	writeJSON(w, http.StatusOK, map[string]string{"status": "shutdown_initiated"})
}

func (s *Server) addToQueue(name string) {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	for _, e := range s.queue {
		if e.Name == name {
			return
		}
	}
	s.queue = append(s.queue, queueEntry{Name: name, Joined: time.Now().UnixMilli()})
}

func (s *Server) removeFromQueue(name string) {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	for i, e := range s.queue {
		if e.Name == name {
			s.queue = append(s.queue[:i], s.queue[i+1:]...)
			return
		}
	}
}
