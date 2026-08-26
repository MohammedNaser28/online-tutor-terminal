package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Persistent user-activity logging under DATA_DIR:
//
//	data/events.log                 — one JSON object per line
//	data/transcripts/<id>-<ts>.log  — raw PTY output per session
var (
	eventLogMu sync.Mutex
	eventLog   *os.File
)

// InitDataDir creates the data layout and opens the event log. Safe to
// call once at server startup.
func InitDataDir(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "transcripts"), 0755); err != nil {
		return fmt.Errorf("create transcripts dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "events.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open events log: %w", err)
	}
	eventLog = f
	return nil
}

// LogEvent appends a structured activity record for a student.
// detail may be empty; it is carried verbatim.
func LogEvent(student, token, event, detail string) {
	if eventLog == nil {
		return
	}
	entry, err := json.Marshal(map[string]string{
		"ts":      time.Now().UTC().Format(time.RFC3339),
		"event":   event,
		"student": student,
		"token":   token,
		"detail":  detail,
	})
	if err != nil {
		return
	}
	eventLogMu.Lock()
	defer eventLogMu.Unlock()
	_, _ = eventLog.Write(append(entry, '\n'))
}

// OpenTranscript creates (or reopens) the raw PTY capture file for a
// student session. Returns nil if logging is unavailable — callers must
// tolerate a nil writer.
func OpenTranscript(dataDir, studentID string) *os.File {
	if dataDir == "" {
		return nil
	}
	name := fmt.Sprintf("%s-%d.log", studentID, time.Now().Unix())
	f, err := os.OpenFile(filepath.Join(dataDir, "transcripts", name),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil
	}
	return f
}

// ─── legacy leveled stdout logging ──────────────────────────────────────────

var logLevel string

func initLogLevel(level string) {
	logLevel = level
}

func logDebug(format string, v ...any) {
	if logLevel == "debug" {
		log.Printf(format, v...)
	}
}

func logInfo(format string, v ...any) {
	if logLevel != "error" && logLevel != "warn" {
		log.Printf(format, v...)
	}
}

func logWarn(format string, v ...any) {
	if logLevel != "error" {
		log.Printf(format, v...)
	}
}

func logError(format string, v ...any) {
	log.Printf(format, v...)
}
