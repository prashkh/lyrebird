// Package handoff produces a self-contained directory describing the current
// state + history of a tracked folder, intended to be handed off to another AI.
package handoff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/prashkh/lyrebird/internal/config"
	"github.com/prashkh/lyrebird/internal/gitstore"
	"github.com/prashkh/lyrebird/internal/session"
)

// Package generates a handoff at outDir. Returns the absolute outDir.
// If outDir is empty, a default under .lyrebird/handoffs/ is used.
func Package(repo *config.Repo, outDir string) (string, error) {
	if outDir == "" {
		stamp := time.Now().UTC().Format("2006-01-02T150405Z")
		outDir = filepath.Join(repo.HandoffsDir, "handoff-"+stamp)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	gs := gitstore.New(repo)
	ss := session.New(repo)

	// 1. Files: copy the current working tree using `git archive` to respect ignores.
	if err := exportFiles(gs, repo, filepath.Join(outDir, "files")); err != nil {
		return "", fmt.Errorf("exporting files: %w", err)
	}

	// 2. Sessions: copy session JSONs verbatim.
	if err := copyDir(repo.SessionsDir, filepath.Join(outDir, "sessions")); err != nil {
		return "", fmt.Errorf("copying sessions: %w", err)
	}

	// 3. Timeline: machine-readable event log derived from git + sessions.
	timeline, err := buildTimeline(gs, ss)
	if err != nil {
		return "", err
	}
	tl, _ := json.MarshalIndent(timeline, "", "  ")
	if err := os.WriteFile(filepath.Join(outDir, "timeline.json"), tl, 0o644); err != nil {
		return "", err
	}

	// 4. HANDOFF.md: deterministic human-readable summary.
	handoffMd := renderHandoffMD(repo, timeline)
	if err := os.WriteFile(filepath.Join(outDir, "HANDOFF.md"), []byte(handoffMd), 0o644); err != nil {
		return "", err
	}

	// 5. CONTEXT.md: LLM-targeted intro.
	ctxMd := renderContextMD(repo)
	if err := os.WriteFile(filepath.Join(outDir, "CONTEXT.md"), []byte(ctxMd), 0o644); err != nil {
		return "", err
	}

	abs, _ := filepath.Abs(outDir)
	return abs, nil
}

// TimelineEvent is one entry in the chronological event log.
type TimelineEvent struct {
	Timestamp     time.Time `json:"timestamp"`
	SnapshotHash  string    `json:"snapshot_hash"`
	Subject       string    `json:"subject"`
	FilesChanged  []string  `json:"files_changed,omitempty"`
	SessionID     string    `json:"session_id,omitempty"`
	TurnID        string    `json:"turn_id,omitempty"`
	Agent         string    `json:"agent,omitempty"`
	UserPrompt    string    `json:"user_prompt,omitempty"`
}

func buildTimeline(gs *gitstore.Store, ss *session.Store) ([]TimelineEvent, error) {
	entries, err := gs.Log(0)
	if err != nil {
		return nil, err
	}
	var out []TimelineEvent
	// Reverse so oldest first — easier for an AI consumer.
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		t, _ := time.Parse(time.RFC3339, e.Date)
		ev := TimelineEvent{
			Timestamp:    t,
			SnapshotHash: e.Hash,
			Subject:      e.Subject,
		}
		if files, err := gs.FilesChanged(e.Hash); err == nil {
			ev.FilesChanged = files
		}
		if sess, turn, _ := ss.FindByCommit(e.Hash); sess != nil && turn != nil {
			ev.SessionID = sess.SessionID
			ev.TurnID = turn.TurnID
			ev.Agent = sess.Agent
			ev.UserPrompt = turn.UserPrompt
		}
		out = append(out, ev)
	}
	return out, nil
}

func renderHandoffMD(repo *config.Repo, timeline []TimelineEvent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Handoff: %s\n\n", repo.Config.FolderName)
	fmt.Fprintf(&b, "Generated: %s\n\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "Tracked since: %s\n\n", repo.Config.Created)

	// Session summary block.
	type sessSummary struct {
		ID          string
		Agent       string
		Started     time.Time
		LastTurn    time.Time
		Turns       int
		FirstPrompt string
	}
	sessions := map[string]*sessSummary{}
	for _, ev := range timeline {
		if ev.SessionID == "" {
			continue
		}
		s, ok := sessions[ev.SessionID]
		if !ok {
			s = &sessSummary{
				ID:          ev.SessionID,
				Agent:       ev.Agent,
				Started:     ev.Timestamp,
				FirstPrompt: ev.UserPrompt,
			}
			sessions[ev.SessionID] = s
		}
		s.Turns++
		s.LastTurn = ev.Timestamp
	}

	fmt.Fprintf(&b, "## Summary\n\n")
	fmt.Fprintf(&b, "- Total snapshots: %d\n", len(timeline))
	fmt.Fprintf(&b, "- AI sessions: %d\n", len(sessions))
	fmt.Fprintf(&b, "- Files in current state: see `files/` directory\n\n")

	// Recent sessions.
	if len(sessions) > 0 {
		fmt.Fprintf(&b, "## Recent sessions\n\n")
		// Sort by LastTurn desc.
		type pair struct{ k string; v *sessSummary }
		pairs := make([]pair, 0, len(sessions))
		for k, v := range sessions {
			pairs = append(pairs, pair{k, v})
		}
		for i := 0; i < len(pairs); i++ {
			for j := i + 1; j < len(pairs); j++ {
				if pairs[j].v.LastTurn.After(pairs[i].v.LastTurn) {
					pairs[i], pairs[j] = pairs[j], pairs[i]
				}
			}
		}
		max := 10
		if len(pairs) < max {
			max = len(pairs)
		}
		for _, p := range pairs[:max] {
			s := p.v
			summary := s.FirstPrompt
			summary = strings.ReplaceAll(summary, "\n", " ")
			if len(summary) > 120 {
				summary = summary[:120] + "..."
			}
			fmt.Fprintf(&b, "- **%s** (%s, %d turns) — %q\n",
				s.LastTurn.Format("2006-01-02 15:04"),
				s.Agent,
				s.Turns,
				summary)
		}
		fmt.Fprintln(&b)
	}

	// Last 20 events.
	if len(timeline) > 0 {
		fmt.Fprintf(&b, "## Recent activity\n\n")
		max := 20
		if len(timeline) < max {
			max = len(timeline)
		}
		// Tail.
		recent := timeline[len(timeline)-max:]
		// Reverse to newest first.
		for i := len(recent) - 1; i >= 0; i-- {
			e := recent[i]
			fmt.Fprintf(&b, "- `%s` %s — %s",
				e.SnapshotHash[:10],
				e.Timestamp.Format("2006-01-02 15:04"),
				e.Subject)
			if len(e.FilesChanged) > 0 {
				fmt.Fprintf(&b, " *(%s)*", strings.Join(e.FilesChanged, ", "))
			}
			fmt.Fprintln(&b)
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, "## How to use this handoff")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "- Read `CONTEXT.md` for an LLM-targeted intro.")
	fmt.Fprintln(&b, "- Inspect `files/` for the current state of the tracked folder.")
	fmt.Fprintln(&b, "- See `sessions/<session-id>.json` for full chat transcripts of past AI sessions.")
	fmt.Fprintln(&b, "- See `timeline.json` for a machine-readable chronological event log.")
	return b.String()
}

func renderContextMD(repo *config.Repo) string {
	return fmt.Sprintf(`# Context for the receiving AI

You are picking up work in a folder named %q that another AI agent was previously
working on. The previous work has been packaged for you in this handoff directory.

## What's in this directory

- `+"`HANDOFF.md`"+` — Human-readable summary of what was done.
- `+"`files/`"+` — The current state of the folder. This is what the user has on disk.
- `+"`sessions/`"+` — Full chat transcripts of every prior AI session, as JSON.
- `+"`timeline.json`"+` — Machine-readable chronological event log linking each
  file change to the chat thread that caused it.

## How to use this context

1. **Start with `+"`HANDOFF.md`"+`** for the high-level summary.
2. **Read the most recent session(s)** in `+"`sessions/`"+` to understand what
   was attempted, what worked, and what was blocked.
3. **Cross-reference `+"`timeline.json`"+`** if you need to know which session
   produced a particular file.
4. The user will tell you what they want next — your job is to continue from
   where the prior agent left off, with awareness of what has been tried.

## What this handoff does NOT include

- The previous AI's reasoning beyond what is recorded in the transcripts.
- Any external context (terminal output, browser state, slack threads, etc.).

If you need more context, ask the user.
`, repo.Config.FolderName)
}

// exportFiles uses `git archive` to write the tracked working tree into dst.
// This naturally respects .lyreignore via the git excludes mechanism.
func exportFiles(gs *gitstore.Store, repo *config.Repo, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	// First check if there are any commits.
	if !gs.HasCommits() {
		return nil
	}
	cmd := exec.Command("git", "archive", "--format=tar", "HEAD")
	cmd.Env = append(os.Environ(),
		"GIT_DIR="+repo.GitDir,
		"GIT_WORK_TREE="+repo.Root,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	tar := exec.Command("tar", "-x", "-C", dst)
	tar.Stdin = stdout
	tar.Stderr = os.Stderr
	if err := tar.Start(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git archive: %w (%s)", err, stderr.String())
	}
	if err := tar.Wait(); err != nil {
		return fmt.Errorf("tar: %w", err)
	}
	return nil
}

// copyDir copies src/* into dst (recursive). Skips if src doesn't exist.
func copyDir(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
