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

func RunCheckScript(rootfsPath string, levelID int) (bool, error) {
	scriptPath := filepath.Join(rootfsPath, "rootfs", "tmp", fmt.Sprintf("level%d", levelID), "check.sh")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return false, nil
	}
	cmd := exec.Command("/bin/bash", scriptPath)
	cmd.Dir = filepath.Join(rootfsPath, "rootfs", "tmp", fmt.Sprintf("level%d", levelID))
	cmd.Stdin = nil
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
		}
	}()

	for i := 0; i < 1200; i++ {
		if session.RootfsPath != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if session.RootfsPath == "" {
		return
	}

	reqFile := filepath.Join(session.RootfsPath, "rootfs", "tmp", ".qo-challenge-req")
	respFile := filepath.Join(session.RootfsPath, "rootfs", "tmp", ".qo-challenge-resp")

	for {
		data, err := os.ReadFile(reqFile)
		if err != nil {
			continue
		}

		_ = os.WriteFile(reqFile, []byte{}, 0644)

		action := strings.TrimSpace(string(data))
		if action == "" {
			continue
		}
		log.Printf("challenge req session=%s action=%q", session.Token, action)
		var resp []byte

		switch action {
		case "quest":
			if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
				current := session.Challenge.Current()
				resp, _ = json.Marshal(map[string]any{
					"story":    session.Title,
					"question": current.Question,
					"level":    session.Challenge.CurrentLevel + 1,
					"total":    session.Challenge.Total(),
				})
			} else {
				resp, _ = json.Marshal(map[string]string{"question": "No challenge loaded."})
			}
		case "hint":
			if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
				current := session.Challenge.Current()
				hint := current.Hint
				if hint == "" {
					hint = "No hint available."
				}
				resp, _ = json.Marshal(map[string]string{"hint": hint})
			} else {
				resp, _ = json.Marshal(map[string]string{"hint": "No hint available."})
			}
		case "go":
			if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
				passed, err := RunCheckScript(session.RootfsPath, session.Challenge.CurrentLevel+1)
				if err != nil {
					resp, _ = json.Marshal(map[string]any{"passed": false, "message": "Check failed: " + err.Error(), "completed": session.Challenge.Completed})
				} else if passed {
					advanced := session.Challenge.Advance()
					msg := "Correct!"
					if advanced {
						msg = fmt.Sprintf("Correct! Advancing to level %d...", session.Challenge.CurrentLevel+1)
					} else {
						msg = "Correct! You completed all levels!"
					}
					resp, _ = json.Marshal(map[string]any{
						"passed":    true,
						"message":   msg,
						"completed": session.Challenge.Completed,
						"next": map[string]any{
							"level":    session.Challenge.CurrentLevel + 1,
							"total":    session.Challenge.Total(),
							"title":    session.Challenge.Current().Title,
							"question": session.Challenge.Current().Question,
							"hint":     session.Challenge.Current().Hint,
						},
					})
				} else {
					resp, _ = json.Marshal(map[string]any{"passed": false, "message": "Not quite right. Try again!", "completed": session.Challenge.Completed})
				}
			} else {
				resp, _ = json.Marshal(map[string]any{"passed": false, "message": "No challenge loaded.", "completed": session.Challenge.Completed})
			}
		case "map":
			if session.Challenge != nil {
				resp, _ = json.Marshal(session.Challenge.Status())
			} else {
				resp, _ = json.Marshal(map[string]any{"levels": []any{}})
			}
		case "status":
			if session.Challenge != nil && len(session.Challenge.Levels) > 0 {
				current := session.Challenge.Current()
				resp, _ = json.Marshal(map[string]any{
					"level":     session.Challenge.CurrentLevel + 1,
					"total":     session.Challenge.Total(),
					"title":     current.Title,
					"completed": session.Challenge.Completed,
				})
			} else {
				resp, _ = json.Marshal(map[string]string{"status": "No challenge loaded."})
			}
		default:
			resp, _ = json.Marshal(map[string]string{"error": "unknown action: " + action})
		}

		_ = os.WriteFile(respFile, append(resp, '\n'), 0644)
		log.Printf("challenge resp session=%s action=%q resp=%s", session.Token, action, string(resp))
	}
}
