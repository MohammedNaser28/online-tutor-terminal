package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type studentLookupResponse struct {
	Found       bool              `json:"found"`
	Live        *sessionInfo      `json:"live"`
	Leaderboard *LeaderboardEntry `json:"leaderboard"`
	Activity    []activityEntry   `json:"activity"`
}

func (s *Server) handleAdminStudent(w http.ResponseWriter, r *http.Request) {
	// Extract ID from /admin/student/<id> or ?id= query
	id := strings.TrimPrefix(r.URL.Path, "/admin/student/")
	if idx := strings.Index(id, "/"); idx >= 0 {
		id = id[:idx]
	}
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	// Also support ?student= for convenience
	if id == "" {
		id = r.URL.Query().Get("student")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "student id required"})
		return
	}

	// Live session if any
	var live *sessionInfo
	if tok, ok := s.manager.LookupByStudentID(id); ok {
		if ses, ok := s.manager.GetSession(tok); ok {
			ses.mu.Lock()
			st := ses.State().String()
			ses.mu.Unlock()
			live = &sessionInfo{
				Token:   ses.Token,
				Name:    ses.StudentID,
				State:   st,
				Created: ses.CreatedAt.UnixMilli(),
				IP:      ses.IP,
			}
		}
	}

	// Historical leaderboard entry
	var lb *LeaderboardEntry
	if s.leaderboardStore != nil {
		if e, ok := s.leaderboardStore.Get(id); ok {
			cp := *e
			lb = &cp
		}
	}

	// Full activity history for this student (scan whole events.log)
	activity := readStudentActivity(s.config.DataDir, id, 200)

	found := live != nil || lb != nil || len(activity) > 0
	// Also consider join_attempt events where student field matches
	if !found {
		// Check if any activity exists at all (including failed joins where student field is set)
		if len(activity) > 0 {
			found = true
		}
	}

	writeJSON(w, http.StatusOK, studentLookupResponse{
		Found:       found,
		Live:        live,
		Leaderboard: lb,
		Activity:    activity,
	})
}

func readStudentActivity(dataDir, studentID string, limit int) []activityEntry {
	candidates := []string{}
	if dataDir != "" {
		candidates = append(candidates, filepath.Join(dataDir, "events.log"))
	}
	candidates = append(candidates, "events.log", filepath.Join("data", "events.log"))

	var eventsPath string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			eventsPath = p
			break
		}
	}
	if eventsPath == "" {
		return nil
	}
	f, err := os.Open(eventsPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var all []activityEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e activityEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if e.Student != studentID {
			continue
		}
		all = append(all, e)
	}
	// Keep newest first, limit
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	// Reverse to newest first for display
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all
}
