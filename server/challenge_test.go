package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ─── helpers ────────────────────────────────────────────────────────────────

func createRootfs(t *testing.T, levels []int) (string, func()) {
	t.Helper()
	rootfs, err := os.MkdirTemp("", "qo-test-rootfs-*")
	if err != nil {
		t.Fatalf("mkdtemp rootfs: %v", err)
	}
	tmp := filepath.Join(rootfs, "rootfs", "tmp")
	proc := filepath.Join(rootfs, "rootfs", "proc")
	if err := os.MkdirAll(tmp, 0755); err != nil {
		os.RemoveAll(rootfs)
		t.Fatalf("mkdir tmp: %v", err)
	}
	if err := os.MkdirAll(proc, 0755); err != nil {
		os.RemoveAll(rootfs)
		t.Fatalf("mkdir proc: %v", err)
	}
	for _, id := range levels {
		if err := os.MkdirAll(filepath.Join(tmp, levelDir(id)), 0755); err != nil {
			os.RemoveAll(rootfs)
			t.Fatalf("mkdir level%d: %v", id, err)
		}
	}
	return rootfs, func() { os.RemoveAll(rootfs) }
}

func levelDir(id int) string {
	return "level" + itoa(id)
}

// ─── ChallengeState ─────────────────────────────────────────────────────────

func TestNewChallengeState_Empty(t *testing.T) {
	cs := NewChallengeState([]ChallengeLevel{})
	if cs.Total() != 0 {
		t.Errorf("expected 0 total, got %d", cs.Total())
	}
	if cs.CurrentLevel != 0 {
		t.Errorf("expected current level 0, got %d", cs.CurrentLevel)
	}
	if cs.Completed {
		t.Error("expected not completed")
	}
}

func TestNewChallengeState_WithLevels(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One", Question: "Q1", Hint: "H1"},
		{ID: 2, Title: "Two", Question: "Q2"},
	}
	cs := NewChallengeState(levels)
	if cs.Total() != 2 {
		t.Errorf("expected 2 total, got %d", cs.Total())
	}
	if cs.Current().Title != "One" {
		t.Errorf("expected current 'One', got %q", cs.Current().Title)
	}
}

func TestChallengeState_Current_Bounds(t *testing.T) {
	cs := NewChallengeState(nil)
	if cs.Current().ID != 0 {
		t.Errorf("expected zero-value level when empty, got ID=%d", cs.Current().ID)
	}

	cs = NewChallengeState([]ChallengeLevel{{ID: 1}})
	cs.CurrentLevel = 99
	if cs.Current().ID != 0 {
		t.Errorf("expected zero-value when current out of range")
	}
}

func TestChallengeState_Progress(t *testing.T) {
	cs := NewChallengeState(nil)
	if p := cs.Progress(); p != 0 {
		t.Errorf("expected 0 progress for empty, got %d", p)
	}

	cs = NewChallengeState([]ChallengeLevel{{ID: 1}, {ID: 2}})
	if p := cs.Progress(); p != 0 {
		t.Errorf("expected 0 progress at level 0, got %d", p)
	}
}

func TestChallengeState_Advance(t *testing.T) {
	levels := []ChallengeLevel{{ID: 1}, {ID: 2}}
	cs := NewChallengeState(levels)

	if ok := cs.Advance(); !ok {
		t.Error("expected advance to return true")
	}
	if cs.CurrentLevel != 1 {
		t.Errorf("expected level 1, got %d", cs.CurrentLevel)
	}
	if cs.Completed {
		t.Error("should not be completed yet")
	}

	if ok := cs.Advance(); ok {
		t.Error("expected advance to return false on last level")
	}
	if !cs.Completed {
		t.Error("expected completed after advancing past last level")
	}
}

func TestChallengeState_Advance_SingleLevel(t *testing.T) {
	cs := NewChallengeState([]ChallengeLevel{{ID: 1}})
	if ok := cs.Advance(); ok {
		t.Error("expected false advancing single level")
	}
	if !cs.Completed {
		t.Error("expected completed after single level")
	}
}

func TestChallengeState_Status(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One"},
		{ID: 2, Title: "Two"},
	}
	cs := NewChallengeState(levels)
	cs.Advance()

	st := cs.Status()
	if v := st["current_level"].(int); v != 1 {
		t.Errorf("expected current_level 1, got %d", v)
	}
	if v := st["total_levels"].(int); v != 2 {
		t.Errorf("expected total_levels 2, got %d", v)
	}
	if v := st["current_title"].(string); v != "Two" {
		t.Errorf("expected current_title 'Two', got %q", v)
	}
	levelsRaw, ok := st["levels"].([]map[string]any)
	if !ok {
		t.Fatal("expected levels as []map[string]any")
	}
	if len(levelsRaw) != 2 {
		t.Fatalf("expected 2 level entries, got %d", len(levelsRaw))
	}
	if !levelsRaw[0]["completed"].(bool) {
		t.Error("expected level 0 to be completed")
	}
	if levelsRaw[1]["completed"].(bool) {
		t.Error("expected level 1 to not be completed")
	}
}

// ─── normalizeMeta ──────────────────────────────────────────────────────────

func TestNormalizeMeta_SingleQuestion(t *testing.T) {
	meta := &ChallengeMetadata{
		Title:       "Test",
		Question:    "The question",
		DefaultHint: "Try harder",
	}
	normalizeMeta(meta)
	if len(meta.Levels) != 1 {
		t.Fatalf("expected 1 level, got %d", len(meta.Levels))
	}
	if meta.Levels[0].ID != 1 {
		t.Errorf("expected ID 1, got %d", meta.Levels[0].ID)
	}
	if meta.Levels[0].Title != "Test" {
		t.Errorf("expected title 'Test', got %q", meta.Levels[0].Title)
	}
	if meta.Levels[0].Hint != "Try harder" {
		t.Errorf("expected hint 'Try harder', got %q", meta.Levels[0].Hint)
	}
}

func TestNormalizeMeta_ExplicitLevels(t *testing.T) {
	meta := &ChallengeMetadata{
		Levels: []ChallengeLevel{
			{ID: 10, Title: "Ten", Question: "Q?", Hint: "H!"},
			{Title: "Twenty"}, // no ID — should get sequential
		},
		Question: "fallback",
	}
	normalizeMeta(meta)
	if len(meta.Levels) != 2 {
		t.Fatalf("expected 2 levels, got %d", len(meta.Levels))
	}
	if meta.Levels[0].ID != 10 {
		t.Errorf("expected ID 10, got %d", meta.Levels[0].ID)
	}
	if meta.Levels[0].Question != "Q?" {
		t.Errorf("expected 'Q?', got %q", meta.Levels[0].Question)
	}
	if meta.Levels[1].ID != 2 {
		t.Errorf("expected ID 2 (sequential), got %d", meta.Levels[1].ID)
	}
	if meta.Levels[1].Question != "fallback" {
		t.Errorf("expected 'fallback', got %q", meta.Levels[1].Question)
	}
	if meta.Levels[0].Hint != "H!" {
		t.Errorf("expected 'H!', got %q", meta.Levels[0].Hint)
	}
}

// ─── loadCheckScripts ───────────────────────────────────────────────────────

func TestLoadCheckScripts_ReadsInitKeepsCheck(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	levelDir := filepath.Join(rootfs, "rootfs", "tmp", levelDir(1))

	checkContent := "#!/bin/bash\necho check"
	initContent := "#!/bin/bash\necho init"

	if err := os.WriteFile(filepath.Join(levelDir, "check.sh"), []byte(checkContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(levelDir, "init.sh"), []byte(initContent), 0644); err != nil {
		t.Fatal(err)
	}

	levels := []ChallengeLevel{{ID: 1}}
	if err := loadCheckScripts(rootfs, levels); err != nil {
		t.Fatalf("loadCheckScripts: %v", err)
	}

	if levels[0].CheckScript != "" {
		t.Errorf("expected empty CheckScript (stays in sandbox), got %q", levels[0].CheckScript)
	}
	if levels[0].InitScript != initContent {
		t.Errorf("expected init.sh content %q, got %q", initContent, levels[0].InitScript)
	}

	if _, err := os.Stat(filepath.Join(levelDir, "check.sh")); err != nil {
		t.Error("check.sh should remain in sandbox for local execution")
	}
	if _, err := os.Stat(filepath.Join(levelDir, "init.sh")); !os.IsNotExist(err) {
		t.Error("init.sh should be deleted from sandbox")
	}
}

func TestLoadCheckScripts_MissingFiles(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	levels := []ChallengeLevel{{ID: 1}}
	if err := loadCheckScripts(rootfs, levels); err != nil {
		t.Fatalf("loadCheckScripts (no scripts): %v", err)
	}
	if levels[0].CheckScript != "" {
		t.Errorf("expected empty CheckScript, got %q", levels[0].CheckScript)
	}
	if levels[0].InitScript != "" {
		t.Errorf("expected empty InitScript, got %q", levels[0].InitScript)
	}
}

// ─── runInitScript ──────────────────────────────────────────────────────────

func TestRunInitScript_ExecutesInSandbox(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	levelDir := filepath.Join(rootfs, "rootfs", "tmp", levelDir(1))

	level := ChallengeLevel{
		ID:         1,
		InitScript: "touch marker.txt\necho 'hello' > greeting.txt",
	}

	if err := runInitScript(rootfs, level); err != nil {
		t.Fatalf("runInitScript: %v", err)
	}

	if _, err := os.Stat(filepath.Join(levelDir, "marker.txt")); os.IsNotExist(err) {
		t.Error("marker.txt should exist after init.sh")
	}
	data, err := os.ReadFile(filepath.Join(levelDir, "greeting.txt"))
	if err != nil {
		t.Fatalf("read greeting.txt: %v", err)
	}
	if strings.TrimSpace(string(data)) != "hello" {
		t.Errorf("expected 'hello', got %q", strings.TrimSpace(string(data)))
	}
}

func TestRunInitScript_EmptySkips(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	level := ChallengeLevel{ID: 1, InitScript: ""}
	if err := runInitScript(rootfs, level); err != nil {
		t.Fatalf("expected nil for empty init script: %v", err)
	}
}

// ─── RunCheckScript ─────────────────────────────────────────────────────────

func TestRunCheckScript_Basic(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	passed, _, err := RunCheckScript(rootfs, 1, "", "exit 0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passed {
		t.Error("expected passed=true for exit 0")
	}

	passed, _, err = RunCheckScript(rootfs, 1, "", "exit 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if passed {
		t.Error("expected passed=false for exit 1")
	}
}

func TestRunCheckScript_EmptyScript(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	passed, _, err := RunCheckScript(rootfs, 1, "", "")
	if err != nil {
		t.Fatalf("expected nil error: %v", err)
	}
	if passed {
		t.Error("expected passed=false for empty script")
	}
}

func TestRunCheckScript_StdinPassthrough(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	// stdinInput is appended to the script and executed as additional shell
	// commands. A script that does not exit can be followed by stdinInput lines.
	script := "true"
	passed, _, err := RunCheckScript(rootfs, 1, "exit 42", script)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if passed {
		t.Error("expected passed=false when stdinInput exits 42")
	}
}

// ─── runValidator ────────────────────────────────────────────────────────────

func TestValidator_Flag(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	level := ChallengeLevel{
		ID: 1,
		Validator: &Validator{
			Type:  ValidatorFlag,
			Value: "correct_flag",
		},
	}

	passed, err := runValidator(rootfs, level, "correct_flag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passed {
		t.Error("expected true for matching flag")
	}

	passed, err = runValidator(rootfs, level, "wrong_flag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if passed {
		t.Error("expected false for wrong flag")
	}
}

func TestValidator_FileExists(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	levelDir := filepath.Join(rootfs, "rootfs", "tmp", levelDir(1))
	if err := os.WriteFile(filepath.Join(levelDir, "exist.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	level := ChallengeLevel{
		ID: 1,
		Validator: &Validator{
			Type: ValidatorFileExists,
			Path: "exist.txt",
		},
	}
	passed, err := runValidator(rootfs, level, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passed {
		t.Error("expected true for existing file")
	}

	level.Validator.Path = "nonexistent.txt"
	passed, err = runValidator(rootfs, level, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if passed {
		t.Error("expected false for nonexistent file")
	}
}

func TestValidator_FileNotExists(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	levelDir := filepath.Join(rootfs, "rootfs", "tmp", levelDir(1))
	if err := os.WriteFile(filepath.Join(levelDir, "present.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	level := ChallengeLevel{
		ID: 1,
		Validator: &Validator{
			Type: ValidatorFileNotExists,
			Path: "gone.txt",
		},
	}
	passed, err := runValidator(rootfs, level, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passed {
		t.Error("expected true for non-existing file")
	}

	level.Validator.Path = "present.txt"
	passed, err = runValidator(rootfs, level, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if passed {
		t.Error("expected false for existing file")
	}
}

func TestValidator_FileContains(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	levelDir := filepath.Join(rootfs, "rootfs", "tmp", levelDir(1))
	if err := os.WriteFile(filepath.Join(levelDir, "data.txt"), []byte("the secret flag is hidden"), 0644); err != nil {
		t.Fatal(err)
	}

	level := ChallengeLevel{
		ID: 1,
		Validator: &Validator{
			Type:  ValidatorFileContains,
			Path:  "data.txt",
			Value: "secret flag",
		},
	}
	passed, err := runValidator(rootfs, level, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passed {
		t.Error("expected true when file contains value")
	}

	level.Validator.Value = "nonexistent"
	passed, err = runValidator(rootfs, level, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if passed {
		t.Error("expected false when file doesn't contain value")
	}
}

func TestValidator_FilePerms(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	levelDir := filepath.Join(rootfs, "rootfs", "tmp", levelDir(1))
	if err := os.WriteFile(filepath.Join(levelDir, "secret.txt"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	level := ChallengeLevel{
		ID: 1,
		Validator: &Validator{
			Type: ValidatorFilePerms,
			Path: "secret.txt",
			Mode: "600",
		},
	}
	passed, err := runValidator(rootfs, level, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passed {
		t.Error("expected true for matching mode 600")
	}

	level.Validator.Mode = "644"
	passed, err = runValidator(rootfs, level, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if passed {
		t.Error("expected false for wrong mode 644 vs 600")
	}
}

func TestValidator_UnknownType(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	level := ChallengeLevel{
		ID: 1,
		Validator: &Validator{
			Type: "bogus_type",
		},
	}
	_, err := runValidator(rootfs, level, "")
	if err == nil {
		t.Fatal("expected error for unknown validator type")
	}
}

func TestValidator_Nil(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	level := ChallengeLevel{ID: 1, Validator: nil}
	_, err := runValidator(rootfs, level, "")
	if err == nil {
		t.Fatal("expected error for nil validator")
	}
}

// ─── processExists ──────────────────────────────────────────────────────────

func TestProcessExists_NotFound(t *testing.T) {
	rootfs, cleanup := createRootfs(t, nil)
	defer cleanup()

	if processExists(rootfs, "nonexistent_process_xyz123") {
		t.Error("expected false for nonexistent process")
	}
}

func TestProcessExists_Found(t *testing.T) {
	rootfs, cleanup := createRootfs(t, nil)
	defer cleanup()

	proc := filepath.Join(rootfs, "rootfs", "proc")
	pidDir := filepath.Join(proc, "42")
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		t.Fatal(err)
	}
	cmdline := filepath.Join(pidDir, "cmdline")
	if err := os.WriteFile(cmdline, []byte("/usr/bin/mydaemon\x00--flag\x00"), 0644); err != nil {
		t.Fatal(err)
	}

	if !processExists(rootfs, "mydaemon") {
		t.Error("expected true for mydaemon")
	}
	if !processExists(rootfs, "usr/bin/mydaemon") {
		t.Error("expected true for usr/bin/mydaemon")
	}
}

func TestProcessExists_EmptyProc(t *testing.T) {
	rootfs, cleanup := createRootfs(t, nil)
	defer cleanup()

	// Add a non-directory entry in /proc (like a file).
	proc := filepath.Join(rootfs, "rootfs", "proc")
	if err := os.WriteFile(filepath.Join(proc, "stat"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	if processExists(rootfs, "anything") {
		t.Error("expected false when only non-dir entries exist in /proc")
	}
}

func TestProcessExists_MissingProc(t *testing.T) {
	rootfs, err := os.MkdirTemp("", "qo-test-noproc-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(rootfs)

	// No /proc directory at all.
	if processExists(rootfs, "anything") {
		t.Error("expected false when /proc is missing")
	}
}

// ─── DiscoverLevelsFromRootfs ───────────────────────────────────────────────

func TestDiscoverLevelsFromRootfs_FindsLevels(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1, 2, 3})
	defer cleanup()

	tmp := filepath.Join(rootfs, "rootfs", "tmp")
	l1 := filepath.Join(tmp, levelDir(1))
	l2 := filepath.Join(tmp, levelDir(2))

	if err := os.WriteFile(filepath.Join(l1, "question.txt"), []byte("Q1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l1, "hint.txt"), []byte("H1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l2, "question.txt"), []byte("Q2"), 0644); err != nil {
		t.Fatal(err)
	}
	// l3 has no files — should still appear with empty fields.

	levels, err := DiscoverLevelsFromRootfs(rootfs)
	if err != nil {
		t.Fatalf("DiscoverLevelsFromRootfs: %v", err)
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}

	if levels[0].ID != 1 {
		t.Errorf("expected ID 1, got %d", levels[0].ID)
	}
	if levels[0].Question != "Q1" {
		t.Errorf("expected Q1, got %q", levels[0].Question)
	}
	if levels[0].Hint != "H1" {
		t.Errorf("expected H1, got %q", levels[0].Hint)
	}

	if levels[1].ID != 2 {
		t.Errorf("expected ID 2, got %d", levels[1].ID)
	}
	if levels[1].Question != "Q2" {
		t.Errorf("expected Q2, got %q", levels[1].Question)
	}

	if levels[2].ID != 3 {
		t.Errorf("expected ID 3, got %d", levels[2].ID)
	}
}

func TestDiscoverLevelsFromRootfs_NonLevelDirsIgnored(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	tmp := filepath.Join(rootfs, "rootfs", "tmp")
	if err := os.MkdirAll(filepath.Join(tmp, "not-a-level"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "level_extra"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "level1", "question.txt"), []byte("Q1"), 0644); err != nil {
		t.Fatal(err)
	}

	levels, err := DiscoverLevelsFromRootfs(rootfs)
	if err != nil {
		t.Fatalf("DiscoverLevelsFromRootfs: %v", err)
	}
	if len(levels) != 1 {
		t.Fatalf("expected 1 level, got %d", len(levels))
	}
}

func TestDiscoverLevelsFromRootfs_NoLevels(t *testing.T) {
	rootfs, cleanup := createRootfs(t, nil)
	defer cleanup()

	_, err := DiscoverLevelsFromRootfs(rootfs)
	if err == nil {
		t.Fatal("expected error when no levels exist")
	}
	if !strings.Contains(err.Error(), "no level directories found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ─── pollChallengeRequests ──────────────────────────────────────────────────

func setupPollTest(t *testing.T, levels []ChallengeLevel) (*Server, *Session, string, func()) {
	t.Helper()

	levelIDs := make([]int, len(levels))
	for i, l := range levels {
		levelIDs[i] = l.ID
	}
	if len(levelIDs) == 0 {
		levelIDs = nil
	}
	rootfs, cleanup := createRootfs(t, levelIDs)
	// Create the tmp dir under rootfs where req/resp files live.
	tmp := filepath.Join(rootfs, "rootfs", "tmp")

	cfg := &Config{
		QoBinaryPath:  createTempFile(t, "qo"),
		ArchivePath:   createTempFile(t, "archive"),
		Password:      "pass",
		Key:           "key",
		MaxConcurrent: 10,
	}
	srv := NewServer(cfg)

	session, err := srv.manager.NewSession("test-student")
	if err != nil {
		cleanup()
		t.Fatalf("NewSession: %v", err)
	}

	session.mu.Lock()
	session.RootfsPath = rootfs
	if len(levels) > 0 {
		session.Challenge = NewChallengeState(levels)
	}
	session.Title = "Test"
	session.mu.Unlock()

	go srv.pollChallengeRequests(session)

	return srv, session, tmp, cleanup
}

func pollAction(t *testing.T, tmpDir string, action string, timeout time.Duration) string {
	t.Helper()

	reqFile := filepath.Join(tmpDir, ".qo-challenge-req")
	respFile := filepath.Join(tmpDir, ".qo-challenge-resp")

	if err := os.WriteFile(reqFile, []byte(action), 0644); err != nil {
		t.Fatalf("write req: %v", err)
	}

	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for poll response")
			return ""
		default:
			data, err := os.ReadFile(respFile)
			if err == nil && len(data) > 0 {
				return strings.TrimRight(string(data), "\n")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestPollChallengeRequests_Quest(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One", Question: "Find the answer"},
		{ID: 2, Title: "Two", Question: "Find the other"},
	}
	srv, session, tmpDir, cleanup := setupPollTest(t, levels)
	defer cleanup()
	defer srv.manager.RemoveSession(session.Token)

	resp := pollAction(t, tmpDir, "quest", 5*time.Second)
	if !strings.Contains(resp, "Level 1/2") {
		t.Errorf("expected Level 1/2 in resp, got %q", resp)
	}
	if !strings.Contains(resp, "Find the answer") {
		t.Errorf("expected question in resp, got %q", resp)
	}
}

func TestPollChallengeRequests_Hint(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One", Question: "Q", Hint: "Look closer"},
	}
	srv, session, tmpDir, cleanup := setupPollTest(t, levels)
	defer cleanup()
	defer srv.manager.RemoveSession(session.Token)

	resp := pollAction(t, tmpDir, "hint", 5*time.Second)
	if !strings.Contains(resp, "Look closer") {
		t.Errorf("expected hint in resp, got %q", resp)
	}
}

func TestPollChallengeRequests_HintMissing(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One", Question: "Q"},
	}
	srv, session, tmpDir, cleanup := setupPollTest(t, levels)
	defer cleanup()
	defer srv.manager.RemoveSession(session.Token)

	resp := pollAction(t, tmpDir, "hint", 5*time.Second)
	if !strings.Contains(resp, "No hint available") {
		t.Errorf("expected 'No hint available', got %q", resp)
	}
}

func TestPollChallengeRequests_GoCorrect(t *testing.T) {
	levels := []ChallengeLevel{
		{
			ID: 1, Title: "One", Question: "Q",
			Validator: &Validator{Type: ValidatorFlag, Value: "correct"},
		},
		{ID: 2, Title: "Two", Question: "Q2"},
	}
	srv, session, tmpDir, cleanup := setupPollTest(t, levels)
	defer cleanup()
	defer srv.manager.RemoveSession(session.Token)

	resp := pollAction(t, tmpDir, "go:correct", 5*time.Second)
	if !strings.Contains(resp, "Correct") {
		t.Errorf("expected 'Correct', got %q", resp)
	}
	if session.Challenge.CurrentLevel != 1 {
		t.Errorf("expected advance to level 1, got %d", session.Challenge.CurrentLevel)
	}
	if session.Score() != 1 {
		t.Errorf("expected score 1, got %d", session.Score())
	}
}

func TestPollChallengeRequests_GoWrong(t *testing.T) {
	levels := []ChallengeLevel{
		{
			ID: 1, Title: "One", Question: "Q",
			Validator: &Validator{Type: ValidatorFlag, Value: "correct"},
		},
	}
	srv, session, tmpDir, cleanup := setupPollTest(t, levels)
	defer cleanup()
	defer srv.manager.RemoveSession(session.Token)

	resp := pollAction(t, tmpDir, "go:wrong", 5*time.Second)
	if !strings.Contains(resp, "Not quite right") {
		t.Errorf("expected 'Not quite right', got %q", resp)
	}
	if session.Challenge.CurrentLevel != 0 {
		t.Errorf("expected no advance, still level 0, got %d", session.Challenge.CurrentLevel)
	}
}

func TestPollChallengeRequests_GoLastLevel(t *testing.T) {
	levels := []ChallengeLevel{
		{
			ID: 1, Title: "One", Question: "Q",
			Validator: &Validator{Type: ValidatorFlag, Value: "ok"},
		},
	}
	srv, session, tmpDir, cleanup := setupPollTest(t, levels)
	defer cleanup()
	defer srv.manager.RemoveSession(session.Token)

	resp := pollAction(t, tmpDir, "go:ok", 5*time.Second)
	if !strings.Contains(resp, "completed all levels") {
		t.Errorf("expected 'completed all levels', got %q", resp)
	}
	if !session.Challenge.Completed {
		t.Error("expected challenge completed")
	}
}

func TestPollChallengeRequests_Map(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One", Question: "Q"},
		{ID: 2, Title: "Two", Question: "Q2"},
	}
	srv, session, tmpDir, cleanup := setupPollTest(t, levels)
	defer cleanup()
	defer srv.manager.RemoveSession(session.Token)

	resp := pollAction(t, tmpDir, "map", 5*time.Second)
	if !strings.Contains(resp, "Challenge Level Map") {
		t.Errorf("expected map header, got %q", resp)
	}
	if !strings.Contains(resp, "Level 1: One") {
		t.Errorf("expected level entry, got %q", resp)
	}
}

func TestPollChallengeRequests_Status(t *testing.T) {
	levels := []ChallengeLevel{
		{ID: 1, Title: "One", Question: "Q"},
	}
	srv, session, tmpDir, cleanup := setupPollTest(t, levels)
	defer cleanup()
	defer srv.manager.RemoveSession(session.Token)

	resp := pollAction(t, tmpDir, "status", 5*time.Second)
	if !strings.Contains(resp, "Level 1/1") {
		t.Errorf("expected 'Level 1/1', got %q", resp)
	}
	if !strings.Contains(resp, "In progress") {
		t.Errorf("expected 'In progress', got %q", resp)
	}
}

func TestPollChallengeRequests_Logo(t *testing.T) {
	levels := []ChallengeLevel{{ID: 1, Title: "One", Question: "Q"}}
	srv, session, tmpDir, cleanup := setupPollTest(t, levels)
	defer cleanup()
	defer srv.manager.RemoveSession(session.Token)

	resp := pollAction(t, tmpDir, "logo", 5*time.Second)
	if !strings.Contains(resp, "QO Logo") {
		t.Errorf("expected 'QO Logo', got %q", resp)
	}
}

func TestPollChallengeRequests_UnknownAction(t *testing.T) {
	levels := []ChallengeLevel{{ID: 1, Title: "One", Question: "Q"}}
	srv, session, tmpDir, cleanup := setupPollTest(t, levels)
	defer cleanup()
	defer srv.manager.RemoveSession(session.Token)

	resp := pollAction(t, tmpDir, "bogus", 5*time.Second)
	if !strings.Contains(resp, "unknown action") {
		t.Errorf("expected 'unknown action', got %q", resp)
	}
}

func TestPollChallengeRequests_NoChallenge(t *testing.T) {
	srv, session, tmpDir, cleanup := setupPollTest(t, nil)
	defer cleanup()
	defer srv.manager.RemoveSession(session.Token)

	// Use a longer timeout because DiscoverLevelsFromRootfs takes ~5s
	// when no level dirs exist.
	for _, action := range []string{"quest", "hint", "go:test", "map", "status"} {
		resp := pollAction(t, tmpDir, action, 10*time.Second)
		if !strings.Contains(resp, "No challenge loaded") {
			t.Errorf("action %q: expected 'No challenge loaded', got %q", action, resp)
		}
	}
}

func TestPollChallengeRequests_EmptyAction(t *testing.T) {
	srv, session, tmpDir, cleanup := setupPollTest(t, nil)
	defer cleanup()
	defer srv.manager.RemoveSession(session.Token)

	// Write an empty request. pollChallengeRequests should skip it.
	reqFile := filepath.Join(tmpDir, ".qo-challenge-req")
	if err := os.WriteFile(reqFile, []byte("  \n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait a bit, then write a real action to make sure the handler still works.
	time.Sleep(200 * time.Millisecond)
	resp := pollAction(t, tmpDir, "help", 5*time.Second)
	if !strings.Contains(resp, "Commands:") {
		t.Errorf("expected Commands:, got %q", resp)
	}
}

func TestPollChallengeRequests_InitScript(t *testing.T) {
	rootfs, cleanup := createRootfs(t, []int{1})
	defer cleanup()

	tmp := filepath.Join(rootfs, "rootfs", "tmp")

	cfg := &Config{
		QoBinaryPath:  createTempFile(t, "qo"),
		ArchivePath:   createTempFile(t, "archive"),
		Password:      "pass",
		Key:           "key",
		MaxConcurrent: 10,
	}
	srv := NewServer(cfg)

	session, err := srv.manager.NewSession("test-student")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	levels := []ChallengeLevel{{
		ID:         1,
		Title:      "InitTest",
		Question:   "Q",
		InitScript: "echo 'created_by_init' > marker.txt",
	}}
	session.mu.Lock()
	session.RootfsPath = rootfs
	session.Challenge = NewChallengeState(levels)
	session.Title = "InitTest"
	session.mu.Unlock()

	go srv.pollChallengeRequests(session)

	resp := pollAction(t, tmp, "quest", 5*time.Second)
	if !strings.Contains(resp, "InitTest") {
		t.Errorf("expected quest text containing 'InitTest', got %q", resp)
	}

	levelDir := filepath.Join(rootfs, "rootfs", "tmp", levelDir(1))
	data, err := os.ReadFile(filepath.Join(levelDir, "marker.txt"))
	if err != nil {
		t.Fatalf("init.sh should have created marker.txt: %v", err)
	}
	if strings.TrimSpace(string(data)) != "created_by_init" {
		t.Errorf("expected 'created_by_init', got %q", strings.TrimSpace(string(data)))
	}
}
