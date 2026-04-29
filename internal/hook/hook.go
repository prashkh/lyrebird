// Package hook handles inbound events from AI agents (Claude Code's PostToolUse,
// later: Codex CLI tail, Cursor SQLite tail). It snapshots the repo and records
// the chat-thread metadata.
package hook

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prashkh/lyrebird/internal/config"
	"github.com/prashkh/lyrebird/internal/gitstore"
	"github.com/prashkh/lyrebird/internal/session"
)

// ClaudeHookPayload is the shape Claude Code sends on PostToolUse via stdin.
// We only declare fields we care about; unknown ones are ignored.
type ClaudeHookPayload struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	HookEventName  string          `json:"hook_event_name"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolResponse   json.RawMessage `json:"tool_response"`
	CWD            string          `json:"cwd"`
}

// HandleClaudeStdin reads a JSON payload from r, finds the lyrebird repo
// containing the relevant working directory, takes a snapshot, and records
// session metadata. If no repo is found, it returns nil silently — this lets
// users install the hook globally and only have some folders tracked.
func HandleClaudeStdin(r *os.File) error {
	dec := json.NewDecoder(r)
	var p ClaudeHookPayload
	if err := dec.Decode(&p); err != nil {
		return fmt.Errorf("parsing hook payload: %w", err)
	}
	return processClaudePayload(p)
}

func processClaudePayload(p ClaudeHookPayload) error {
	// Determine which folder this hook applies to.
	startDir := p.CWD
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	repo, err := config.Open(startDir)
	if err != nil {
		// Not a tracked folder. Silently no-op — this is the "install once,
		// activate per-folder" UX.
		return nil
	}

	// Only act on tools that wrote/edited files. Skip read-only tools.
	if !isWriteTool(p.ToolName) {
		return nil
	}

	// Pull recent context out of the transcript file (best effort).
	userPrompt, assistantText := readRecentTurn(p.TranscriptPath)

	// Generate a turn ID. Claude Code doesn't expose one, so we synthesize.
	turnID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Take the snapshot.
	gs := gitstore.New(repo)
	filesTouched := extractFiles(p.ToolInput, repo.Root)
	subject := fmt.Sprintf("[ai] claude-code %s %s", short(p.SessionID), strings.Join(truncList(filesTouched, 3), " "))
	hash, err := gs.Snapshot(subject)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	// Persist session metadata. We do this even when hash=="" (e.g. .ipynb where
	// the agent edited but git already had it — we still want the chat thread.)
	ss := session.New(repo)
	turn := session.Turn{
		TurnID:        turnID,
		Timestamp:     time.Now().UTC(),
		Tool:          p.ToolName,
		UserPrompt:    userPrompt,
		AssistantText: assistantText,
		FilesChanged:  filesTouched,
		SnapshotHash:  hash,
	}
	// Stash the raw tool input as JSON.
	if len(p.ToolInput) > 0 {
		var raw any
		if json.Unmarshal(p.ToolInput, &raw) == nil {
			turn.ToolInput = raw
		}
	}
	if _, err := ss.AppendTurn(p.SessionID, "claude-code", turn); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func isWriteTool(name string) bool {
	switch name {
	case "Edit", "Write", "NotebookEdit", "MultiEdit":
		return true
	}
	return false
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// truncList returns at most n elements, or the original slice.
func truncList(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// extractFiles pulls likely file paths from a tool input payload.
func extractFiles(input json.RawMessage, repoRoot string) []string {
	if len(input) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(input, &m); err != nil {
		return nil
	}
	var files []string
	for _, key := range []string{"file_path", "notebook_path", "path"} {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				if rel, err := filepath.Rel(repoRoot, s); err == nil && !strings.HasPrefix(rel, "..") {
					files = append(files, rel)
				} else {
					files = append(files, s)
				}
			}
		}
	}
	return files
}

// readRecentTurn parses Claude Code's transcript JSONL and extracts
// the most recent user prompt and most recent assistant text (best effort).
// Format: each line is a JSON object with at least a "type" and "message" field.
func readRecentTurn(path string) (userPrompt, assistantText string) {
	if path == "" {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	type entry struct {
		Type    string          `json:"type"`
		Message json.RawMessage `json:"message"`
	}
	type msgContent struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	var lastUser, lastAssistant string
	for scanner.Scan() {
		var e entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if e.Type != "user" && e.Type != "assistant" {
			continue
		}
		var msg msgContent
		if err := json.Unmarshal(e.Message, &msg); err != nil {
			continue
		}
		text := extractText(msg.Content)
		if text == "" {
			continue
		}
		switch msg.Role {
		case "user":
			lastUser = text
		case "assistant":
			lastAssistant = text
		}
	}
	return lastUser, lastAssistant
}

// extractText pulls human-readable text out of Anthropic-style "content" blocks.
// Content can be a string or an array of blocks like {"type":"text","text":"..."}.
func extractText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	// Try plain string first.
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s
	}
	// Try array of blocks.
	var blocks []map[string]any
	if err := json.Unmarshal(content, &blocks); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, b := range blocks {
		if t, _ := b["type"].(string); t == "text" {
			if v, ok := b["text"].(string); ok {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(v)
			}
		}
	}
	return sb.String()
}

// errSkipped is returned silently when no repo applies.
var errSkipped = errors.New("not in a tracked folder")
