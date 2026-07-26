package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ChallengeLevel struct {
	ID       int    `json:"id" yaml:"id"`
	Title    string `json:"title" yaml:"title"`
	Question string `json:"question" yaml:"question"`
	Hint     string `json:"hint,omitempty" yaml:"hint,omitempty"`
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
	CurrentLevel int
	Levels       []ChallengeLevel
	Completed    bool
}

func NewChallengeState(levels []ChallengeLevel) *ChallengeState {
	if len(levels) == 0 {
		return &ChallengeState{Levels: []ChallengeLevel{}}
	}
	return &ChallengeState{CurrentLevel: 0, Levels: levels}
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

func (c *ChallengeState) Advance() bool {
	if c.CurrentLevel+1 >= len(c.Levels) {
		c.Completed = true
		return false
	}
	c.CurrentLevel++
	return true
}

func (c *ChallengeState) Status() map[string]any {
	levels := make([]map[string]any, len(c.Levels))
	for i, lv := range c.Levels {
		levels[i] = map[string]any{
			"id":       lv.ID,
			"title":    lv.Title,
			"completed": i < c.CurrentLevel,
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

func RunCheckScript(rootfsPath string, levelID int, stdinInput string) (bool, error) {
	scriptPath := filepath.Join(rootfsPath, "rootfs", "tmp", fmt.Sprintf("level%d", levelID), "check.sh")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return false, nil
	}
	cmd := exec.Command("/bin/bash", scriptPath)
	cmd.Dir = filepath.Join(rootfsPath, "rootfs", "tmp", fmt.Sprintf("level%d", levelID))
	if stdinInput != "" {
		cmd.Stdin = strings.NewReader(stdinInput + "\n")
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode() == 0, nil
		}
		return false, err
	}
	return true, nil
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
				var id int
				if _, err := fmt.Sscanf(name, "level%d", &id); err != nil || id <= 0 {
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
			goAnswer := ""
			if strings.HasPrefix(action, "go:") {
				goAnswer = action[3:]
				action = "go"
			}

			switch action {
			case "quest":
				if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
					current := session.Challenge.Current()
					resp = []byte(fmt.Sprintf("\033[96m━━━ Level %d/%d\033[0m  %s\n%s",
						session.Challenge.CurrentLevel+1, session.Challenge.Total(),
						session.Title, current.Question))
				} else {
					resp = []byte("\033[33mNo challenge loaded.\033[0m")
				}
			case "hint":
				if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
					current := session.Challenge.Current()
					hint := current.Hint
					if hint == "" {
						hint = "No hint available."
					}
					resp = []byte(fmt.Sprintf("\033[33m💡 Hint: %s\033[0m", hint))
				} else {
					resp = []byte("\033[33mNo hint available.\033[0m")
				}
			case "go":
				if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
					passed, err := RunCheckScript(session.RootfsPath, session.Challenge.CurrentLevel+1, goAnswer)
					if err != nil {
						resp = []byte(fmt.Sprintf("\033[31m❌ Check failed: %s\033[0m", err.Error()))
					} else if passed {
						session.IncrementScore()
						advanced := session.Challenge.Advance()
						if advanced {
							resp = []byte(fmt.Sprintf("\033[32m✅ Correct! Advancing to level %d...\033[0m", session.Challenge.CurrentLevel+1))
						} else {
							resp = []byte(fmt.Sprintf("\033[32m🎉 Correct! You completed all levels!\033[0m"))
						}
					} else {
						resp = []byte("\033[31m❌ Not quite right. Try again!\033[0m")
					}
				} else {
					resp = []byte("\033[33mNo challenge loaded.\033[0m")
				}
			case "map":
				if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
					st := session.Challenge.Status()
					cur, curOk := st["current_level"].(int)
					tot, totOk := st["total_levels"].(int)
					title, titleOk := st["current_title"].(string)
					if !curOk || !totOk || !titleOk {
						resp = []byte("\033[31m❌ Failed to read challenge status.\033[0m")
					} else {
						resp = []byte(fmt.Sprintf("\033[96m━━━ Map %d/%d\033[0m  %s", cur+1, tot, title))
					}
				} else {
					resp = []byte("\033[33mNo challenge loaded.\033[0m")
				}
			case "status":
				if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
					current := session.Challenge.Current()
					status := "\033[33mIn progress\033[0m"
					if session.Challenge.Completed {
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
				resp = []byte("\033[1mCommands: quest, hint, go, map, status, logo, help\033[0m")
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
