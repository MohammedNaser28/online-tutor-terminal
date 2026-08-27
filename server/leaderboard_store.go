package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type LeaderboardEntry struct {
	StudentID    string `json:"student_id"`
	Solved       int    `json:"solved"`
	LastSolvedAt int64  `json:"last_solved_at"`
	LastIP       string `json:"last_ip"`
	LastSeenAt   int64  `json:"last_seen_at"`
}

type LeaderboardStore struct {
	mu   sync.RWMutex
	m    map[string]*LeaderboardEntry
	path string
}

func NewLeaderboardStore(dataDir string) *LeaderboardStore {
	ls := &LeaderboardStore{
		m:    make(map[string]*LeaderboardEntry),
		path: filepath.Join(dataDir, "leaderboard.json"),
	}
	ls.Load()
	return ls
}

func (ls *LeaderboardStore) Load() {
	data, err := os.ReadFile(ls.path)
	if err != nil {
		ls.replayEventsLog()
		return
	}
	var entries map[string]*LeaderboardEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		ls.replayEventsLog()
		return
	}
	ls.mu.Lock()
	ls.m = entries
	if ls.m == nil {
		ls.m = make(map[string]*LeaderboardEntry)
	}
	ls.mu.Unlock()
	ls.replayEventsLog()
}

func (ls *LeaderboardStore) replayEventsLog() {
	dataDir := filepath.Dir(ls.path)
	eventsPath := filepath.Join(dataDir, "events.log")
	if _, err := os.Stat(eventsPath); err != nil {
		return
	}
	f, err := os.Open(eventsPath)
	if err != nil {
		return
	}
	defer f.Close()

	solvedSets := make(map[string]map[string]bool)
	lastTime := make(map[string]int64)
	lastIP := make(map[string]string)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e map[string]string
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		sid := e["student"]
		if sid == "" {
			continue
		}
		ts, _ := time.Parse(time.RFC3339, e["ts"])
		millis := ts.UnixMilli()
		if millis == 0 {
			millis = time.Now().UnixMilli()
		}
		if millis > lastTime[sid] {
			lastTime[sid] = millis
			if detail := e["detail"]; detail != "" {
				if idx := strings.Index(detail, "ip="); idx >= 0 {
					ip := detail[idx+3:]
					if sp := strings.Index(ip, " "); sp >= 0 {
						ip = ip[:sp]
					}
					lastIP[sid] = ip
				}
			}
		}
		if e["event"] == "go_attempt" && strings.Contains(e["detail"], "passed=true") {
			lvl := ""
			if idx := strings.Index(e["detail"], "level="); idx >= 0 {
				rest := e["detail"][idx+6:]
				if end := strings.Index(rest, " "); end >= 0 {
					lvl = rest[:end]
				} else {
					lvl = rest
				}
			}
			if lvl == "" {
				continue
			}
			if solvedSets[sid] == nil {
				solvedSets[sid] = make(map[string]bool)
			}
			solvedSets[sid][lvl] = true
		}
	}

	ls.mu.Lock()
	defer ls.mu.Unlock()
	for sid, set := range solvedSets {
		entry, ok := ls.m[sid]
		if !ok {
			entry = &LeaderboardEntry{StudentID: sid}
			ls.m[sid] = entry
		}
		entry.Solved = len(set)
		if t, ok := lastTime[sid]; ok && t > entry.LastSolvedAt {
			entry.LastSolvedAt = t
		}
		if ip, ok := lastIP[sid]; ok {
			entry.LastIP = ip
		}
		if t, ok := lastTime[sid]; ok {
			entry.LastSeenAt = t
		}
	}
	for sid, t := range lastTime {
		if _, ok := ls.m[sid]; !ok {
			ls.m[sid] = &LeaderboardEntry{StudentID: sid, LastSeenAt: t, LastIP: lastIP[sid]}
		} else if ls.m[sid].LastSeenAt < t {
			ls.m[sid].LastSeenAt = t
			if ip, ok := lastIP[sid]; ok {
				ls.m[sid].LastIP = ip
			}
		}
	}
}

func (ls *LeaderboardStore) persistLocked() {
	data, err := json.MarshalIndent(ls.m, "", "  ")
	if err != nil {
		return
	}
	tmp := ls.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	_ = os.Rename(tmp, ls.path)
}

func (ls *LeaderboardStore) Inc(studentID, ip string) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	e, ok := ls.m[studentID]
	if !ok {
		e = &LeaderboardEntry{StudentID: studentID}
		ls.m[studentID] = e
	}
	e.Solved++
	e.LastSolvedAt = time.Now().UnixMilli()
	e.LastSeenAt = e.LastSolvedAt
	if ip != "" {
		e.LastIP = ip
	}
	ls.persistLocked()
}

func (ls *LeaderboardStore) Touch(studentID, ip string) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	e, ok := ls.m[studentID]
	if !ok {
		e = &LeaderboardEntry{StudentID: studentID}
		ls.m[studentID] = e
	}
	e.LastSeenAt = time.Now().UnixMilli()
	if ip != "" {
		e.LastIP = ip
	}
	ls.persistLocked()
}

func (ls *LeaderboardStore) Get(studentID string) (*LeaderboardEntry, bool) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	e, ok := ls.m[studentID]
	if !ok {
		return nil, false
	}
	cp := *e
	return &cp, true
}

func (ls *LeaderboardStore) GetAll() []LeaderboardEntry {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	out := make([]LeaderboardEntry, 0, len(ls.m))
	for _, v := range ls.m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Solved != out[j].Solved {
			return out[i].Solved > out[j].Solved
		}
		if out[i].LastSolvedAt != out[j].LastSolvedAt {
			return out[i].LastSolvedAt < out[j].LastSolvedAt
		}
		return out[i].StudentID < out[j].StudentID
	})
	return out
}
