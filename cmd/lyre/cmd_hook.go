package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/prashkh/lyrebird/internal/config"
	"github.com/prashkh/lyrebird/internal/hook"
	"github.com/prashkh/lyrebird/internal/session"
)

func cmdHook(args []string) error {
	fs := flag.NewFlagSet("hook", flag.ExitOnError)
	_ = fs.Parse(args)
	// Read JSON from stdin (Claude Code's hook contract).
	if err := hook.HandleClaudeStdin(os.Stdin); err != nil {
		// Hook handlers should fail silently to stderr (Claude Code is configured
		// to ignore non-zero exits unless we want them to block).
		fmt.Fprintln(os.Stderr, "lyre hook:", err)
		return nil
	}
	return nil
}

// cmdInstallHook patches ~/.claude/settings.json to add a PostToolUse hook
// that calls `lyre hook`. Idempotent.
func cmdInstallHook(args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	exe, err := os.Executable()
	if err != nil {
		exe = "lyre"
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}

	// Load existing settings (empty object if file missing).
	var settings map[string]any
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parsing existing %s: %w", settingsPath, err)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		settings = map[string]any{}
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
			return err
		}
	} else {
		return err
	}

	// Navigate / create hooks.PostToolUse.
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}
	post, _ := hooks["PostToolUse"].([]any)

	// Check whether our hook is already registered.
	cmdStr := exe + " hook"
	already := false
	for _, m := range post {
		mm, _ := m.(map[string]any)
		hh, _ := mm["hooks"].([]any)
		for _, h := range hh {
			hm, _ := h.(map[string]any)
			if c, _ := hm["command"].(string); c == cmdStr {
				already = true
			}
		}
	}
	if already {
		fmt.Println("Lyrebird hook already installed in", settingsPath)
		return nil
	}

	entry := map[string]any{
		"matcher": "Edit|Write|NotebookEdit|MultiEdit",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": cmdStr,
			},
		},
	}
	post = append(post, entry)
	hooks["PostToolUse"] = post

	// Write back with stable key ordering for diff-friendliness.
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(settingsPath, out, 0o644); err != nil {
		return err
	}
	fmt.Printf("Installed Lyrebird PostToolUse hook in %s\n", settingsPath)
	fmt.Println("Restart Claude Code to pick up the new hook.")
	return nil
}

// cmdSessions lists recent sessions in the current repo.
func cmdSessions(args []string) error {
	repo, err := config.Open(".")
	if err != nil {
		return err
	}
	ss := session.New(repo)
	list, err := ss.List()
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Println("(no sessions yet — install the hook with `lyre install-hook`)")
		return nil
	}
	for _, s := range list {
		summary := s.PromptSummary()
		summary = strings.ReplaceAll(summary, "\n", " ")
		if len(summary) > 80 {
			summary = summary[:80] + "..."
		}
		fmt.Printf("%s  %s  %d turns  %s\n", s.UpdatedAt.Format("2006-01-02 15:04"), shortID(s.SessionID), len(s.Turns), summary)
	}
	return nil
}

// cmdSession prints the full transcript + files touched for one session.
func cmdSession(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: lyre session <id>")
	}
	id := args[0]
	repo, err := config.Open(".")
	if err != nil {
		return err
	}
	ss := session.New(repo)
	list, err := ss.List()
	if err != nil {
		return err
	}
	var match *session.Session
	for _, s := range list {
		if strings.HasPrefix(s.SessionID, id) {
			match = s
			break
		}
	}
	if match == nil {
		return fmt.Errorf("no session matching %q", id)
	}
	fmt.Printf("Session: %s (%s)\n", match.SessionID, match.Agent)
	fmt.Printf("Started: %s\n", match.StartedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Turns:   %d\n\n", len(match.Turns))
	turns := append([]session.Turn(nil), match.Turns...)
	sort.SliceStable(turns, func(i, j int) bool {
		return turns[i].Timestamp.Before(turns[j].Timestamp)
	})
	for i, t := range turns {
		fmt.Printf("── Turn %d (%s) — tool=%s ", i+1, t.Timestamp.Format("15:04:05"), t.Tool)
		if t.SnapshotHash != "" {
			fmt.Printf("snapshot=%s", t.SnapshotHash[:12])
		}
		fmt.Println()
		if len(t.FilesChanged) > 0 {
			fmt.Printf("Files: %s\n", strings.Join(t.FilesChanged, ", "))
		}
		if t.UserPrompt != "" {
			fmt.Println("User:")
			fmt.Println(indent(t.UserPrompt, "  "))
		}
		if t.AssistantText != "" {
			fmt.Println("Assistant:")
			fmt.Println(indent(t.AssistantText, "  "))
		}
		fmt.Println()
	}
	return nil
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
