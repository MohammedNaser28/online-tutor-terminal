package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type activityEntry struct {
	TS      string `json:"ts"`
	Event   string `json:"event"`
	Student string `json:"student"`
	Token   string `json:"token"`
	Detail  string `json:"detail"`
}

func (s *Server) handleAdminActivity(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	filterStudent := r.URL.Query().Get("student")
	filterEvent := r.URL.Query().Get("event")

	// Determine events.log path: try DATA_DIR, then legacy ./events.log
	candidates := []string{}
	if s.config.DataDir != "" {
		candidates = append(candidates, filepath.Join(s.config.DataDir, "events.log"))
	}
	candidates = append(candidates, "events.log")
	candidates = append(candidates, filepath.Join("data", "events.log"))

	var eventsPath string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			eventsPath = p
			break
		}
	}
	if eventsPath == "" {
		writeJSON(w, http.StatusOK, []activityEntry{})
		return
	}

	entries := tailActivityLog(eventsPath, limit*3) // over-read to allow filtering

	// Filter
	filtered := make([]activityEntry, 0, limit)
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if filterStudent != "" && e.Student != filterStudent {
			continue
		}
		if filterEvent != "" && e.Event != filterEvent {
			continue
		}
		filtered = append(filtered, e)
		if len(filtered) >= limit {
			break
		}
	}
	// Reverse to chronological (oldest first) for display? Keep newest first for admin log
	// We already iterate newest first, so filtered is newest first
	writeJSON(w, http.StatusOK, filtered)
}

func tailActivityLog(path string, maxLines int) []activityEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	// For simplicity and correctness, read all lines (events.log is small for exams < 10k lines)
	// If it grows large, we could seek from end, but keep simple for now.
	var all []activityEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e activityEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if e.Event == "" {
			continue
		}
		// Normalize detail trimming
		e.Detail = strings.TrimSpace(e.Detail)
		all = append(all, e)
	}
	if len(all) <= maxLines {
		return all
	}
	return all[len(all)-maxLines:]
}
