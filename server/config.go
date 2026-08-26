package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port          int
	EventCode     string
	AdminSecret   string
	QoBinaryPath  string
	ArchivePath   string
	Password      string
	Key           string
	MaxConcurrent int
	GracePeriod   time.Duration
	IdleTimeout   time.Duration
	DataDir       string
	QoDuration    string
	LogLevel      string
}

func LoadConfig() (*Config, error) {
	port, err := envInt("PORT", 8080)
	if err != nil {
		return nil, fmt.Errorf("PORT: %w", err)
	}

	eventCode := os.Getenv("EVENT_CODE")
	if eventCode == "" {
		return nil, fmt.Errorf("EVENT_CODE is required")
	}

	adminSecret := os.Getenv("ADMIN_SECRET")
	if adminSecret == "" {
		return nil, fmt.Errorf("ADMIN_SECRET is required")
	}

	qoBin := os.Getenv("QO_BINARY_PATH")
	if qoBin == "" {
		return nil, fmt.Errorf("QO_BINARY_PATH is required")
	}
	if _, err := os.Stat(qoBin); err != nil {
		return nil, fmt.Errorf("QO_BINARY_PATH %q: %w", qoBin, err)
	}

	archivePath := os.Getenv("ARCHIVE_PATH")
	if archivePath == "" {
		return nil, fmt.Errorf("ARCHIVE_PATH is required")
	}
	if _, err := os.Stat(archivePath); err != nil {
		return nil, fmt.Errorf("ARCHIVE_PATH %q: %w", archivePath, err)
	}

	maxConcurrent, err := envInt("MAX_CONCURRENT", 8)
	if err != nil {
		return nil, fmt.Errorf("MAX_CONCURRENT: %w", err)
	}
	if maxConcurrent < 1 {
		return nil, fmt.Errorf("MAX_CONCURRENT must be >= 1, got %d", maxConcurrent)
	}

	graceSecs, err := envInt("GRACE_PERIOD", 45)
	if err != nil {
		return nil, fmt.Errorf("GRACE_PERIOD: %w", err)
	}
	if graceSecs < 1 {
		return nil, fmt.Errorf("GRACE_PERIOD must be >= 1, got %d", graceSecs)
	}

	idleSecs, err := envInt("IDLE_TIMEOUT", 600)
	if err != nil {
		return nil, fmt.Errorf("IDLE_TIMEOUT: %w", err)
	}
	if idleSecs < 1 {
		return nil, fmt.Errorf("IDLE_TIMEOUT must be >= 1, got %d", idleSecs)
	}

	password := os.Getenv("ARCHIVE_PASSWORD")
	if password == "" {
		return nil, fmt.Errorf("ARCHIVE_PASSWORD is required")
	}

	key := os.Getenv("ARCHIVE_KEY")
	if key == "" {
		return nil, fmt.Errorf("ARCHIVE_KEY is required")
	}

	duration := os.Getenv("QO_DURATION")
	if duration == "" {
		return nil, fmt.Errorf("QO_DURATION is required (e.g. \"90m\", \"2h\")")
	}

	if _, err := time.ParseDuration(duration); err != nil {
		return nil, fmt.Errorf("QO_DURATION %q is not a valid duration: %w", duration, err)
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}

	return &Config{
		Port:          port,
		EventCode:     eventCode,
		AdminSecret:   adminSecret,
		QoBinaryPath:  qoBin,
		ArchivePath:   archivePath,
		Password:      password,
		Key:           key,
		MaxConcurrent: maxConcurrent,
		GracePeriod:   time.Duration(graceSecs) * time.Second,
		IdleTimeout:   time.Duration(idleSecs) * time.Second,
		QoDuration:    duration,
		LogLevel:      logLevel,
	}, nil
}

func envInt(key string, defaultVal int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultVal, nil
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", raw, err)
	}
	return val, nil
}
