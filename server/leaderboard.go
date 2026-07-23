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
	var entries []leaderboardEntry

	s.manager.AllSessions(func(ses *Session) {
		entries = append(entries, leaderboardEntry{
			ID:     ses.StudentID,
			Name:   ses.StudentID,
			Solved: 0,
			Avatar: ses.StudentID[:1],
		})
	})

	if entries == nil {
		entries = []leaderboardEntry{}
	}

	writeJSON(w, http.StatusOK, entries)
}
