package main

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig_Defaults(t *testing.T) {
	qoBin := createTempFile(t, "qo")
	archive := createTempFile(t, "archive")
	os.Clearenv()
	t.Setenv("EVENT_CODE", "test123")
	t.Setenv("ADMIN_SECRET", "admin-secret-123")
	t.Setenv("QO_BINARY_PATH", qoBin)
	t.Setenv("ARCHIVE_PATH", archive)
	t.Setenv("ARCHIVE_PASSWORD", "pass123")
	t.Setenv("ARCHIVE_KEY", "key123")
	t.Setenv("QO_DURATION", "90m")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.MaxConcurrent != 8 {
		t.Errorf("expected max 8, got %d", cfg.MaxConcurrent)
	}
	if cfg.GracePeriod != 45*time.Second {
		t.Errorf("expected 45s grace, got %v", cfg.GracePeriod)
	}
	if cfg.IdleTimeout != 600*time.Second {
		t.Errorf("expected 600s idle, got %v", cfg.IdleTimeout)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected info log, got %s", cfg.LogLevel)
	}
}

func TestLoadConfig_MissingEventCode(t *testing.T) {
	qoBin := createTempFile(t, "qo")
	archive := createTempFile(t, "archive")
	os.Clearenv()
	t.Setenv("ADMIN_SECRET", "admin-secret-123")
	t.Setenv("QO_BINARY_PATH", qoBin)
	t.Setenv("ARCHIVE_PATH", archive)
	t.Setenv("ARCHIVE_PASSWORD", "pass123")
	t.Setenv("ARCHIVE_KEY", "key123")
	t.Setenv("QO_DURATION", "90m")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing EVENT_CODE")
	}
}

func TestLoadConfig_MissingAdminSecret(t *testing.T) {
	qoBin := createTempFile(t, "qo")
	archive := createTempFile(t, "archive")
	os.Clearenv()
	t.Setenv("EVENT_CODE", "test123")
	t.Setenv("QO_BINARY_PATH", qoBin)
	t.Setenv("ARCHIVE_PATH", archive)
	t.Setenv("ARCHIVE_PASSWORD", "pass123")
	t.Setenv("ARCHIVE_KEY", "key123")
	t.Setenv("QO_DURATION", "90m")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing ADMIN_SECRET")
	}
}

func TestLoadConfig_MissingQoBinary(t *testing.T) {
	archive := createTempFile(t, "archive")
	os.Clearenv()
	t.Setenv("EVENT_CODE", "test123")
	t.Setenv("ADMIN_SECRET", "admin-secret-123")
	t.Setenv("QO_BINARY_PATH", "/nonexistent/qo")
	t.Setenv("ARCHIVE_PATH", archive)
	t.Setenv("ARCHIVE_PASSWORD", "pass123")
	t.Setenv("ARCHIVE_KEY", "key123")
	t.Setenv("QO_DURATION", "90m")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing QO_BINARY_PATH")
	}
}

func TestLoadConfig_MissingArchive(t *testing.T) {
	qoBin := createTempFile(t, "qo")
	os.Clearenv()
	t.Setenv("EVENT_CODE", "test123")
	t.Setenv("ADMIN_SECRET", "admin-secret-123")
	t.Setenv("QO_BINARY_PATH", qoBin)
	t.Setenv("ARCHIVE_PATH", "/nonexistent/archive")
	t.Setenv("ARCHIVE_PASSWORD", "pass123")
	t.Setenv("ARCHIVE_KEY", "key123")
	t.Setenv("QO_DURATION", "90m")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing ARCHIVE_PATH")
	}
}

func TestLoadConfig_MissingDuration(t *testing.T) {
	qoBin := createTempFile(t, "qo")
	archive := createTempFile(t, "archive")
	os.Clearenv()
	t.Setenv("EVENT_CODE", "test123")
	t.Setenv("ADMIN_SECRET", "admin-secret-123")
	t.Setenv("QO_BINARY_PATH", qoBin)
	t.Setenv("ARCHIVE_PATH", archive)
	t.Setenv("ARCHIVE_PASSWORD", "pass123")
	t.Setenv("ARCHIVE_KEY", "key123")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing QO_DURATION")
	}
}

func TestLoadConfig_CustomValues(t *testing.T) {
	qoBin := createTempFile(t, "qo")
	archive := createTempFile(t, "archive")
	os.Clearenv()
	t.Setenv("EVENT_CODE", "custom-event")
	t.Setenv("ADMIN_SECRET", "custom-admin-secret")
	t.Setenv("QO_BINARY_PATH", qoBin)
	t.Setenv("ARCHIVE_PATH", archive)
	t.Setenv("QO_DURATION", "2h")
	t.Setenv("ARCHIVE_PASSWORD", "custom-pass")
	t.Setenv("ARCHIVE_KEY", "custom-key")
	t.Setenv("PORT", "9999")
	t.Setenv("MAX_CONCURRENT", "4")
	t.Setenv("GRACE_PERIOD", "60")
	t.Setenv("IDLE_TIMEOUT", "300")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.Port)
	}
	if cfg.MaxConcurrent != 4 {
		t.Errorf("expected max 4, got %d", cfg.MaxConcurrent)
	}
	if cfg.GracePeriod != 60*time.Second {
		t.Errorf("expected 60s grace, got %v", cfg.GracePeriod)
	}
	if cfg.IdleTimeout != 300*time.Second {
		t.Errorf("expected 300s idle, got %v", cfg.IdleTimeout)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected debug log, got %s", cfg.LogLevel)
	}
	if cfg.QoDuration != "2h" {
		t.Errorf("expected 2h duration, got %s", cfg.QoDuration)
	}
	if cfg.AdminSecret != "custom-admin-secret" {
		t.Errorf("expected custom-admin-secret, got %s", cfg.AdminSecret)
	}
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	qoBin := createTempFile(t, "qo")
	archive := createTempFile(t, "archive")
	os.Clearenv()
	t.Setenv("EVENT_CODE", "test123")
	t.Setenv("ADMIN_SECRET", "admin-secret-123")
	t.Setenv("QO_BINARY_PATH", qoBin)
	t.Setenv("ARCHIVE_PATH", archive)
	t.Setenv("QO_DURATION", "90m")
	t.Setenv("ARCHIVE_PASSWORD", "pass123")
	t.Setenv("ARCHIVE_KEY", "key123")
	t.Setenv("PORT", "not-a-number")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid PORT")
	}
}

func createTempFile(t *testing.T, name string) string {
	t.Helper()
	f, err := os.CreateTemp("", name)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}
