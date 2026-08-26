package main

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

type SessionState int

const (
	SessionPending SessionState = iota
	SessionActive
	SessionOrphaned
	SessionClosed
)

func (s SessionState) String() string {
	switch s {
	case SessionPending:
		return "pending"
	case SessionActive:
		return "active"
	case SessionOrphaned:
		return "orphaned"
	case SessionClosed:
		return "closed"
	default:
		return "unknown"
	}
}

type Session struct {
	Token      string
	StudentID  string
	IP         string
	Title      string
	Difficulty string
	RootfsPath string
	CreatedAt  time.Time

	state      atomic.Int64
	lastActive atomic.Int64
	score      atomic.Int64

	mu        sync.Mutex
	Cmd       *exec.Cmd
	Term      *os.File
	Challenge *ChallengeState
}

func NewSession(studentID, ip string) *Session {
	s := &Session{
		Token:     uuid.NewString(),
		StudentID: studentID,
		IP:        ip,
		CreatedAt: time.Now(),
	}
	s.state.Store(int64(SessionPending))
	s.lastActive.Store(time.Now().UnixNano())
	return s
}

func (s *Session) State() SessionState {
	return SessionState(s.state.Load())
}

func (s *Session) SetState(st SessionState) {
	s.state.Store(int64(st))
	if st == SessionActive || st == SessionPending {
		s.lastActive.Store(time.Now().UnixNano())
	}
}

func (s *Session) LastActive() time.Time {
	return time.Unix(0, s.lastActive.Load())
}

func (s *Session) Touch() {
	s.lastActive.Store(time.Now().UnixNano())
}

func (s *Session) Score() int {
	return int(s.score.Load())
}

func (s *Session) SetScore(n int) {
	s.score.Store(int64(n))
	s.Touch()
}

func (s *Session) IncrementScore() int {
	n := int(s.score.Add(1))
	s.Touch()
	return n
}

func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Store(int64(SessionClosed))

	if s.Term != nil {
		s.Term.Close()
		s.Term = nil
	}
	if s.Cmd != nil && s.Cmd.Process != nil {
		s.Cmd.Process.Kill()
		s.Cmd.Wait()
		s.Cmd = nil
	}
}

type SessionManager struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	byStudentID map[string]string
	capacity    int
}

func NewSessionManager(capacity int) *SessionManager {
	return &SessionManager{
		sessions:    make(map[string]*Session),
		byStudentID: make(map[string]string),
		capacity:    capacity,
	}
}

func (m *SessionManager) NewSession(studentID, ip string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existingToken, ok := m.byStudentID[studentID]; ok {
		if s, exists := m.sessions[existingToken]; exists && s.State() != SessionClosed {
			return nil, fmt.Errorf("student %s already has an active session", studentID)
		}
	}

	if m.countActiveLocked() >= m.capacity {
		return nil, fmt.Errorf("concurrency cap reached (%d/%d)", m.capacity, m.capacity)
	}

	s := NewSession(studentID, ip)
	m.sessions[s.Token] = s
	m.byStudentID[studentID] = s.Token
	return s, nil
}

func (m *SessionManager) GetSession(token string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[token]
	return s, ok
}

func (m *SessionManager) LookupByStudentID(sid string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tok, ok := m.byStudentID[sid]
	return tok, ok
}

func (m *SessionManager) GetSessionByStudentID(studentID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if token, ok := m.byStudentID[studentID]; ok {
		s, exists := m.sessions[token]
		return s, exists
	}
	return nil, false
}

func (m *SessionManager) RemoveSession(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[token]
	if !ok {
		return
	}

	delete(m.sessions, token)
	delete(m.byStudentID, s.StudentID)
}

func (m *SessionManager) CountActive() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.countActiveLocked()
}

func (m *SessionManager) countActiveLocked() int {
	count := 0
	for _, s := range m.sessions {
		st := s.State()
		if st == SessionPending || st == SessionActive || st == SessionOrphaned {
			count++
		}
	}
	return count
}

func (m *SessionManager) SetSessionState(token string, state SessionState) error {
	m.mu.RLock()
	s, ok := m.sessions[token]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", token)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State() == SessionClosed && state != SessionClosed {
		return fmt.Errorf("session %s is already closed", token)
	}

	s.SetState(state)
	return nil
}

func (m *SessionManager) AllSessions(fn func(*Session)) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		fn(s)
	}
}
