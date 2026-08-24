package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type ValidatorType string

const (
	ValidatorFlag           ValidatorType = "flag"
	ValidatorProcessDead    ValidatorType = "process_dead"
	ValidatorProcessRunning ValidatorType = "process_running"
	ValidatorFileExists     ValidatorType = "file_exists"
	ValidatorFileNotExists  ValidatorType = "file_not_exists"
	ValidatorFileContains   ValidatorType = "file_contains"
	ValidatorFilePerms      ValidatorType = "file_permissions"
)

type Validator struct {
	Type  ValidatorType `json:"type"`
	Value string        `json:"value,omitempty"`
	Path  string        `json:"path,omitempty"`
	Name  string        `json:"name,omitempty"`
	Mode  string        `json:"mode,omitempty"`
}

type ChallengeLevel struct {
	ID           int        `json:"id"`
	Title        string     `json:"title"`
	Question     string     `json:"question"`
	Hint         string     `json:"hint,omitempty"`
	Validator    *Validator `json:"validator,omitempty"`
	CheckScript  string     `json:"-"` // loaded from file, kept server-side only
	InitScript   string     `json:"-"` // setup script, deleted after first run
}

type ChallengeMetadata struct {
	Title       string            `json:"title,omitempty" yaml:"title,omitempty"`
	Difficulty  string            `json:"difficulty,omitempty" yaml:"difficulty,omitempty"`
	Story       string            `json:"story,omitempty" yaml:"story,omitempty"`
	Question    string            `json:"question,omitempty" yaml:"question,omitempty"`
	Levels      []ChallengeLevel  `json:"levels,omitempty" yaml:"levels,omitempty"`
	DefaultHint string            `json:"default_hint,omitempty" yaml:"default_hint,omitempty"`
}

type ChallengeState struct {
	CurrentLevel    int
	Levels          []ChallengeLevel
	Completed       bool
	initRan         map[int]bool // levels whose init.sh has been run
	completedLevels map[int]bool // levels that have been completed
}

func NewChallengeState(levels []ChallengeLevel) *ChallengeState {
	if len(levels) == 0 {
		return &ChallengeState{Levels: []ChallengeLevel{}, initRan: map[int]bool{}, completedLevels: map[int]bool{}}
	}
	return &ChallengeState{CurrentLevel: 0, Levels: levels, initRan: map[int]bool{}, completedLevels: map[int]bool{}}
}

func (c *ChallengeState) Current() ChallengeLevel {
	if c.CurrentLevel < 0 || c.CurrentLevel >= len(c.Levels) {
		return ChallengeLevel{}
	}
	return c.Levels[c.CurrentLevel]
}

func (c *ChallengeState) Total() int {
	return len(c.Levels)
}

func (c *ChallengeState) Progress() int {
	if len(c.Levels) == 0 {
		return 0
	}
	return c.CurrentLevel
}

func (c *ChallengeState) SetLevel(levelID int) bool {
	for i, lv := range c.Levels {
		if lv.ID == levelID || i+1 == levelID {
			c.CurrentLevel = i
			return true
		}
	}
	return false
}

func (c *ChallengeState) MarkCompleted(levelID int) {
	if c.completedLevels == nil {
		c.completedLevels = make(map[int]bool)
	}
	c.completedLevels[levelID] = true

	allDone := true
	for _, lv := range c.Levels {
		if !c.completedLevels[lv.ID] {
			allDone = false
			break
		}
	}
	if allDone && len(c.Levels) > 0 {
		c.Completed = true
	}
}

func (c *ChallengeState) Advance() bool {
	c.MarkCompleted(c.Current().ID)
	if c.CurrentLevel+1 >= len(c.Levels) {
		return false
	}
	c.CurrentLevel++
	return true
}

func (c *ChallengeState) Status() map[string]any {
	levels := make([]map[string]any, len(c.Levels))
	for i, lv := range c.Levels {
		levels[i] = map[string]any{
			"id":        lv.ID,
			"title":     lv.Title,
			"completed": c.completedLevels[lv.ID],
			"unlocked":  true,
			"active":    i == c.CurrentLevel,
		}
	}
	return map[string]any{
		"current_level":   c.CurrentLevel,
		"total_levels":    len(c.Levels),
		"completed":       c.Completed,
		"current_title":   c.Current().Title,
		"levels":          levels,
	}
}

func LoadChallengeMeta(binaryPath, archivePath, password, key string) (*ChallengeMetadata, error) {
	out, err := exec.Command(binaryPath, "meta",
		"-a", archivePath,
		"-p", password,
		"-k", key,
	).Output()
	if err != nil {
		return nil, err
	}

	var meta ChallengeMetadata
	if err := json.Unmarshal(out, &meta); err != nil {
		return nil, err
	}

	normalizeMeta(&meta)
	return &meta, nil
}

func normalizeMeta(meta *ChallengeMetadata) {
	if len(meta.Levels) == 0 && meta.Question != "" {
		meta.Levels = []ChallengeLevel{{
			ID:       1,
			Title:    meta.Title,
			Question: meta.Question,
			Hint:     meta.DefaultHint,
		}}
	}
	for i := range meta.Levels {
		if meta.Levels[i].ID == 0 {
			meta.Levels[i].ID = i + 1
		}
		if meta.Levels[i].Question == "" {
			meta.Levels[i].Question = meta.Question
		}
		if meta.Levels[i].Hint == "" && meta.DefaultHint != "" {
			meta.Levels[i].Hint = meta.DefaultHint
		}
	}
}

func findLevelDir(rootfsPath string, levelID int) string {
	baseTmp := filepath.Join(rootfsPath, "rootfs", "tmp")
	candidates := []string{
		fmt.Sprintf("level%d", levelID),
		fmt.Sprintf("Level-%d", levelID),
		fmt.Sprintf("Level%d", levelID),
		fmt.Sprintf("level-%d", levelID),
	}
	for _, c := range candidates {
		p := filepath.Join(baseTmp, c)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(baseTmp, fmt.Sprintf("level%d", levelID))
}

func parseLevelID(name string) int {
	var id int
	if _, err := fmt.Sscanf(name, "level%d", &id); err == nil && id > 0 {
		return id
	}
	if _, err := fmt.Sscanf(name, "Level-%d", &id); err == nil && id > 0 {
		return id
	}
	if _, err := fmt.Sscanf(name, "Level%d", &id); err == nil && id > 0 {
		return id
	}
	if _, err := fmt.Sscanf(name, "level-%d", &id); err == nil && id > 0 {
		return id
	}
	return 0
}

func loadCheckScripts(rootfsPath string, levels []ChallengeLevel) error {
	for i := range levels {
		lvl := &levels[i]

		// Wait for level directory to exist (archive extraction may still be in progress).
		var lvlDir string
		for attempt := 0; attempt < 30; attempt++ {
			candidate := findLevelDir(rootfsPath, lvl.ID)
			if _, err := os.Stat(candidate); err == nil {
				lvlDir = candidate
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if lvlDir == "" {
			lvlDir = filepath.Join(rootfsPath, "rootfs", "tmp", fmt.Sprintf("level%d", lvl.ID))
		}

		// Read and remove each file with per-file retry — the directory
		// may exist before all its contents are written.
		readFile := func(path, label string, store *string) {
			for attempt := 0; attempt < 10; attempt++ {
				data, err := os.ReadFile(path)
				if err == nil {
					if len(data) > 0 || *store == "" {
						*store = string(data)
					}
					if err := os.Remove(path); err == nil {
						log.Printf("secured %s: level=%d", label, lvl.ID)
					}
					return
				}
				if os.IsNotExist(err) {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
		}

		readFile(filepath.Join(lvlDir, "init.sh"), "init.sh", &lvl.InitScript)
		readFile(filepath.Join(lvlDir, "question.txt"), "question.txt", &lvl.Question)
		readFile(filepath.Join(lvlDir, "hint.txt"), "hint.txt", &lvl.Hint)
	}
	return nil
}

func runInitScript(rootfsPath string, level ChallengeLevel) error {
	if level.InitScript == "" {
		return nil
	}
	cmd := exec.Command("/bin/bash", "-s", "--")
	cmd.Dir = findLevelDir(rootfsPath, level.ID)
	cmd.Stdin = strings.NewReader(level.InitScript + "\n")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("init.sh failed for level %d: %w", level.ID, err)
	}
	log.Printf("init.sh completed for level %d", level.ID)
	return nil
}

func RunCheckScript(rootfsPath string, levelID int, stdinInput string, checkScript string) (bool, string, error) {
	if checkScript == "" {
		return false, "No check script found for this level.", nil
	}

	levelDir := findLevelDir(rootfsPath, levelID)
	chrootPath := filepath.Join(rootfsPath, "rootfs")

	// Go applies Chroot before Chdir, so the working directory must be
	// expressed relative to the inside of the chroot.
	cwd := "/tmp/" + filepath.Base(levelDir)
	if rel, err := filepath.Rel(chrootPath, levelDir); err == nil && !strings.HasPrefix(rel, "..") {
		cwd = "/" + rel
	}

	cmdArgs := []string{"-s", "--"}
	cmd := exec.Command("/bin/bash", cmdArgs...)
	cmd.Dir = cwd
	// Chroot needs real root; skip it when running unprivileged (tests,
	// dev runs) and address the sandbox through the host path instead.
	if os.Geteuid() == 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Chroot: chrootPath,
		}
	} else {
		cmd.Dir = levelDir
	}
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/home/ahmed",
		"USER=ahmed",
		"LOGNAME=ahmed",
		"TERM=xterm",
		"LD_LIBRARY_PATH=/usr/lib:/lib:/lib/x86_64-linux-gnu:/usr/lib/x86_64-linux-gnu:/lib/aarch64-linux-gnu:/usr/lib/aarch64-linux-gnu",
	}

	var sb strings.Builder
	sb.WriteString("set -e\n")
	sb.WriteString(checkScript)
	sb.WriteByte('\n')
	if stdinInput != "" {
		sb.WriteString(stdinInput)
		sb.WriteByte('\n')
	}
	cmd.Stdin = strings.NewReader(sb.String())

	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	err := cmd.Run()
	output := strings.TrimSpace(outBuf.String())
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode() == 0, output, nil
		}
		return false, output, err
	}
	return true, output, nil
}

func runValidator(rootfsPath string, level ChallengeLevel, answer string) (bool, error) {
	v := level.Validator
	if v == nil {
		// No validator defined — caller should fall back to check script.
		return false, fmt.Errorf("no validator")
	}

	levelDir := findLevelDir(rootfsPath, level.ID)

	switch v.Type {
	case ValidatorFlag:
		return answer == v.Value, nil

	case ValidatorProcessDead:
		return !processExists(rootfsPath, v.Name), nil

	case ValidatorProcessRunning:
		return processExists(rootfsPath, v.Name), nil

	case ValidatorFileExists:
		_, err := os.Stat(filepath.Join(levelDir, v.Path))
		return err == nil, nil

	case ValidatorFileNotExists:
		_, err := os.Stat(filepath.Join(levelDir, v.Path))
		return err != nil, nil

	case ValidatorFileContains:
		path := filepath.Join(levelDir, v.Path)
		data, err := os.ReadFile(path)
		if err != nil {
			return false, nil
		}
		return strings.Contains(string(data), v.Value), nil

	case ValidatorFilePerms:
		path := filepath.Join(levelDir, v.Path)
		info, err := os.Stat(path)
		if err != nil {
			return false, nil
		}
		mode := fmt.Sprintf("%o", info.Mode().Perm())
		return mode == v.Mode, nil

	default:
		return false, fmt.Errorf("unknown validator type: %s", v.Type)
	}
}

func processExists(rootfsPath string, name string) bool {
	procDir := filepath.Join(rootfsPath, "rootfs", "proc")
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join(procDir, e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		if strings.Contains(string(cmdline), name) {
			return true
		}
	}
	return false
}

func DiscoverLevelsFromRootfs(rootfsPath string) ([]ChallengeLevel, error) {
	tmpDir := filepath.Join(rootfsPath, "rootfs", "tmp")

	for attempt := 0; attempt < 20; attempt++ {
		entries, err := os.ReadDir(tmpDir)
		if err == nil {
			var levels []ChallengeLevel
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				name := e.Name()
				id := parseLevelID(name)
				if id <= 0 {
					continue
				}

				level := ChallengeLevel{ID: id, Title: name}

				qPath := filepath.Join(tmpDir, name, "question.txt")
				if data, err := os.ReadFile(qPath); err == nil {
					level.Question = string(data)
				}

				hPath := filepath.Join(tmpDir, name, "hint.txt")
				if data, err := os.ReadFile(hPath); err == nil {
					level.Hint = string(data)
				}

				levels = append(levels, level)
			}

			if len(levels) > 0 {
				return levels, nil
			}
		}

		time.Sleep(250 * time.Millisecond)
	}

	return nil, fmt.Errorf("no level directories found in %s", tmpDir)
}

func (s *Server) pollChallengeRequests(session *Session) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic in pollChallengeRequests for %s: %v", session.Token, r)
			if session.RootfsPath != "" {
				_ = os.WriteFile(
					filepath.Join(session.RootfsPath, "rootfs", "tmp", ".qo-challenge-resp"),
					[]byte(fmt.Sprintf("\033[31m❌ Internal error, restarting handler...\033[0m\n")),
					0644,
				)
			}
			// Restart after a short delay so transient panics don't kill the handler permanently.
			time.Sleep(1 * time.Second)
			go s.pollChallengeRequests(session)
		}
	}()

	for i := 0; i < 1200; i++ {
		if session.RootfsPath != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if session.RootfsPath == "" {
		log.Printf("pollChallengeRequests session=%s no RootfsPath after 120s, exiting", session.Token)
		return
	}

	// If challenge was not initialized from metadata (e.g. meta.yaml had no
	// levels or question), fall back to discovering level directories from the
	// rootfs filesystem.  DiscoverLevelsFromRootfs retries internally for ~5s,
	// giving the qo-start process time to extract the archive.
	session.mu.Lock()
	needsDiscovery := session.Challenge == nil || len(session.Challenge.Levels) == 0
	session.mu.Unlock()

	if needsDiscovery {
		levels, err := DiscoverLevelsFromRootfs(session.RootfsPath)
		if err == nil && len(levels) > 0 {
			session.mu.Lock()
			if session.Challenge == nil || len(session.Challenge.Levels) == 0 {
				session.Challenge = NewChallengeState(levels)
				log.Printf("pollChallengeRequests: discovered %d levels from rootfs for session %s", len(levels), session.Token)
			}
			session.mu.Unlock()
		} else {
			log.Printf("pollChallengeRequests: no levels discovered from rootfs for session %s: %v", session.Token, err)
		}
	}

	reqFile := filepath.Join(session.RootfsPath, "rootfs", "tmp", ".qo-challenge-req")
	respFile := filepath.Join(session.RootfsPath, "rootfs", "tmp", ".qo-challenge-resp")

	log.Printf("pollChallengeRequests session=%s started rootfs=%s", session.Token, session.RootfsPath)

	for {
		func() {
			var resp []byte
			defer func() {
				if r := recover(); r != nil {
					log.Printf("panic processing action in session %s: %v", session.Token, r)
					resp = []byte(fmt.Sprintf("\033[31m❌ Error processing command. Try again.\033[0m"))
					_ = os.WriteFile(respFile, append(resp, '\n'), 0644)
				}
			}()

			data, err := os.ReadFile(reqFile)
			if err != nil {
				time.Sleep(50 * time.Millisecond)
				return
			}

			action := strings.TrimSpace(string(data))
			if action == "" {
				time.Sleep(50 * time.Millisecond)
				return
			}

			// Clear the request file immediately after reading so repeated
			// identical commands are not ignored (the shell blocks until it
			// reads a response, so there is no write-side race).
			_ = os.WriteFile(reqFile, []byte{}, 0644)

			log.Printf("challenge req session=%s action=%q", session.Token, action)
			arg := ""
			if idx := strings.IndexByte(action, ':'); idx >= 0 {
				arg = strings.TrimSpace(action[idx+1:])
				action = strings.TrimSpace(action[:idx])
			}

			parseArgLevel := func(s string) int {
				if s == "" {
					return 0
				}
				var num int
				if _, err := fmt.Sscanf(s, "level%d", &num); err == nil && num > 0 {
					return num
				}
				if _, err := fmt.Sscanf(s, "Level-%d", &num); err == nil && num > 0 {
					return num
				}
				if _, err := fmt.Sscanf(s, "level-%d", &num); err == nil && num > 0 {
					return num
				}
				if _, err := fmt.Sscanf(s, "%d", &num); err == nil && num > 0 {
					return num
				}
				return 0
			}

			switch action {
			case "quest":
				if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
					if targetLvl := parseArgLevel(arg); targetLvl > 0 {
						session.Challenge.SetLevel(targetLvl)
						levelFile := filepath.Join(session.RootfsPath, "rootfs", "tmp", ".qo-current-level")
						_ = os.WriteFile(levelFile, []byte(fmt.Sprintf("level%d", targetLvl)), 0644)
					}
					current := session.Challenge.Current()
					levelID := session.Challenge.CurrentLevel + 1

					// Run init.sh on first access to this level.
					if !session.Challenge.initRan[current.ID] && current.InitScript != "" {
						if err := runInitScript(session.RootfsPath, current); err != nil {
							log.Printf("init.sh failed: level=%d err=%v", current.ID, err)
						}
						session.Challenge.initRan[current.ID] = true
					}

					resp = []byte(fmt.Sprintf("\033[96m━━━ Level %d/%d\033[0m  %s\n%s",
						levelID, session.Challenge.Total(),
						current.Title, current.Question))
				} else {
					resp = []byte("\033[33mNo challenge loaded.\033[0m")
				}

			case "level", "select":
				if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
					if targetLvl := parseArgLevel(arg); targetLvl > 0 {
						if session.Challenge.SetLevel(targetLvl) {
							current := session.Challenge.Current()
							resp = []byte(fmt.Sprintf("\033[32mSwitched to Level %d: %s\033[0m\n\n%s",
								session.Challenge.CurrentLevel+1, current.Title, current.Question))
							levelFile := filepath.Join(session.RootfsPath, "rootfs", "tmp", ".qo-current-level")
							_ = os.WriteFile(levelFile, []byte(fmt.Sprintf("level%d", targetLvl)), 0644)
						} else {
							resp = []byte(fmt.Sprintf("\033[31mInvalid level %d. Total levels: %d\033[0m", targetLvl, session.Challenge.Total()))
						}
					} else {
						resp = []byte(fmt.Sprintf("\033[33mCurrent level: %d/%d. Usage: level <number>\033[0m",
							session.Challenge.CurrentLevel+1, session.Challenge.Total()))
					}
				} else {
					resp = []byte("\033[33mNo challenge loaded.\033[0m")
				}

			case "hint":
				if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
					if targetLvl := parseArgLevel(arg); targetLvl > 0 {
						session.Challenge.SetLevel(targetLvl)
					}
					current := session.Challenge.Current()
					hint := current.Hint
					if hint == "" {
						hint = "No hint available."
					}
					resp = []byte(fmt.Sprintf("\033[33m💡 Hint (Level %d): %s\033[0m", session.Challenge.CurrentLevel+1, hint))
				} else {
					resp = []byte("\033[33mNo hint available.\033[0m")
				}

			case "go":
				if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
					if targetLvl := parseArgLevel(arg); targetLvl > 0 {
						session.Challenge.SetLevel(targetLvl)
						arg = ""
					}

					level := session.Challenge.Current()
					var passed bool
					var output string
					var err error

					if level.CheckScript != "" {
						passed, output, err = RunCheckScript(session.RootfsPath, level.ID, arg, level.CheckScript)
					} else if level.Validator != nil {
						passed, err = runValidator(session.RootfsPath, level, arg)
					}

					var outSb strings.Builder
					if output != "" {
						outSb.WriteString(output)
						outSb.WriteByte('\n')
					}

					if err != nil {
						outSb.WriteString(fmt.Sprintf("\033[31m❌ Check error: %s\033[0m", err.Error()))
					} else if passed {
						session.Challenge.MarkCompleted(level.ID)
						session.IncrementScore()
						if session.Challenge.Completed {
							outSb.WriteString("\033[32m🎉 Correct! You've completed all levels!\033[0m")
						} else if session.Challenge.Advance() {
							outSb.WriteString(fmt.Sprintf("\033[32m✅ Correct! Now on level %d/%d\033[0m",
								session.Challenge.CurrentLevel+1, session.Challenge.Total()))
						} else {
							outSb.WriteString("\033[32m✅ Correct!\033[0m")
						}
					} else {
						outSb.WriteString(fmt.Sprintf("\033[31m❌ Not quite right. Level %d incomplete. Try again!\033[0m", level.ID))
					}
					resp = []byte(outSb.String())
				} else {
					resp = []byte("\033[33mNo challenge loaded.\033[0m")
				}

			case "solved":
				// Sent by the in-sandbox __qo_go helper after a local
				// check.sh exits 0 — records completion and score.
				if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
					targetLvl := parseArgLevel(arg)
					var lvl ChallengeLevel
					if targetLvl > 0 {
						found := false
						for _, lv := range session.Challenge.Levels {
							if lv.ID == targetLvl {
								lvl = lv
								found = true
								break
							}
						}
						if !found {
							resp = []byte(fmt.Sprintf("\033[31m❌ Unknown level %d\033[0m", targetLvl))
							break
						}
					} else {
						lvl = session.Challenge.Current()
					}
					session.Challenge.MarkCompleted(lvl.ID)
					session.IncrementScore()
					if !session.Challenge.Completed {
						session.Challenge.Advance()
					}
					log.Printf("challenge solved session=%s level=%d via local check.sh", session.Token, lvl.ID)
					resp = []byte(fmt.Sprintf("\033[32m🎉 Level %d recorded as passed!\033[0m", lvl.ID))
				} else {
					resp = []byte("\033[33mNo challenge loaded.\033[0m")
				}

			case "map":
				if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
					var mapSb strings.Builder
					mapSb.WriteString("\033[96m━━━ Challenge Level Map ━━━\033[0m\n")
					for i, lv := range session.Challenge.Levels {
						statusStr := "\033[33m[Unlocked]\033[0m"
						if session.Challenge.completedLevels[lv.ID] {
							statusStr = "\033[32m[Completed]\033[0m"
						}
						activeMarker := "  "
						if i == session.Challenge.CurrentLevel {
							activeMarker = "\033[96m▶ \033[0m"
						}
						mapSb.WriteString(fmt.Sprintf("%sLevel %d: %s %s\n", activeMarker, lv.ID, lv.Title, statusStr))
					}
					mapSb.WriteString("\n\033[90mTip: Type 'level <number>' or 'quest <number>' to jump to any level.\033[0m")
					resp = []byte(mapSb.String())
				} else {
					resp = []byte("\033[33mNo challenge loaded.\033[0m")
				}

			case "status":
				if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
					current := session.Challenge.Current()
					status := "\033[33mIn progress\033[0m"
					if session.Challenge.completedLevels[current.ID] {
						status = "\033[32m✅ Completed\033[0m"
					}
					resp = []byte(fmt.Sprintf("\033[96mLevel %d/%d\033[0m  %s  %s",
						session.Challenge.CurrentLevel+1, session.Challenge.Total(),
						current.Title, status))
				} else {
					resp = []byte("\033[33mNo challenge loaded.\033[0m")
				}

			case "logo":
				resp = []byte("\033[36mQO Logo\033[0m")

			case "help":
				resp = []byte("\033[1mCommands:\n  quest [n]    - View question for current or level n\n  level <n>    - Switch to level n\n  go           - Run check script for current level\n  hint         - Show hint for current level\n  map          - List all levels\n  status       - Show session progress\033[0m")

			default:
				resp, _ = json.Marshal(map[string]string{"error": "unknown action: " + action})
			}

			if resp != nil {
				_ = os.WriteFile(respFile, append(resp, '\n'), 0644)
				log.Printf("challenge resp session=%s action=%q resp=%s", session.Token, action, string(resp))
			}
		}()
	}
}
