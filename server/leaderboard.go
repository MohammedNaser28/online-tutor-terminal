package main

import (
	"net/http"
)

type leaderboardEntry struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Solved int    `json:"solved"`
	Avatar string `json:"avatar"`
}

func (s *Server) handleLeaderboardPage(w http.ResponseWriter, r *http.Request) {
	serveEmbedded(w, r, "leaderboard.html")
}

func (s *Server) handleLeaderboardData(w http.ResponseWriter, r *http.Request) {
	// Historical leaderboard: all students who ever solved, persisted across
	// restarts. Sorted Solved DESC, LastSolvedAt ASC (earlier achiever wins ties).
	if s.leaderboardStore != nil {
		stored := s.leaderboardStore.GetAll()
		entries := make([]leaderboardEntry, 0, len(stored))
		for _, e := range stored {
			if e.StudentID == "" {
				continue
			}
			avatar := e.StudentID[:1]
			entries = append(entries, leaderboardEntry{
				ID:     e.StudentID,
				Name:   e.StudentID,
				Solved: e.Solved,
				Avatar: avatar,
			})
		}
		// Also include live sessions that haven't solved yet (0 solved) so
		// newcomers appear immediately
		seen := make(map[string]bool)
		for _, e := range entries {
			seen[e.ID] = true
		}
		s.manager.AllSessions(func(ses *Session) {
			if seen[ses.StudentID] {
				return
			}
			entries = append(entries, leaderboardEntry{
				ID:     ses.StudentID,
				Name:   ses.StudentID,
				Solved: ses.Score(),
				Avatar: ses.StudentID[:1],
			})
		})
		writeJSON(w, http.StatusOK, entries)
		return
	}

	var entries []leaderboardEntry
	s.manager.AllSessions(func(ses *Session) {
		entries = append(entries, leaderboardEntry{
			ID:     ses.StudentID,
			Name:   ses.StudentID,
			Solved: ses.Score(),
			Avatar: ses.StudentID[:1],
		})
	})
	if entries == nil {
		entries = []leaderboardEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}
