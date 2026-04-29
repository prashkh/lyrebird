// Package session reads and writes per-AI-session metadata as JSON sidecars.
package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/prashkh/lyrebird/internal/config"
)

// Turn is one user prompt → assistant response → tool calls cycle.
type Turn struct {
	TurnID         string    `json:"turn_id"`
	Timestamp      time.Time `json:"timestamp"`
	Tool           string    `json:"tool"`
	UserPrompt     string    `json:"user_prompt,omitempty"`
	AssistantText  string    `json:"assistant_text,omitempty"`
	ToolInput      any       `json:"tool_input,omitempty"`
	FilesChanged   []string  `json:"files_changed,omitempty"`
	SnapshotHash   string    `json:"snapshot_hash,omitempty"`
}

// Session is the metadata for one AI conversation.
type Session struct {
	SessionID  string    `json:"session_id"`
	Agent      string    `json:"agent"`
	StartedAt  time.Time `json:"started_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Turns      []Turn    `json:"turns"`
	Summary    string    `json:"summary,omitempty"`
}

// Store handles session JSON files under .lyrebird/sessions/.
type Store struct {
	repo *config.Repo
}

func New(r *config.Repo) *Store { return &Store{repo: r} }

// path returns the absolute path for a session's JSON file.
func (s *Store) path(sessionID string) string {
	return filepath.Join(s.repo.SessionsDir, sanitize(sessionID)+".json")
}

// sanitize prevents path-traversal in session IDs from external sources.
func sanitize(id string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", "..", "_", "\x00", "_")
	return r.Replace(id)
}

// Load returns the session if it exists, or a zero value with the ID set.
func (s *Store) Load(sessionID string) (*Session, error) {
	p := s.path(sessionID)
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return &Session{SessionID: sessionID, StartedAt: time.Now().UTC()}, nil
	}
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", p, err)
	}
	return &sess, nil
}

// Save writes the session JSON.
func (s *Store) Save(sess *Session) error {
	if sess.SessionID == "" {
		return errors.New("session has no SessionID")
	}
	if sess.StartedAt.IsZero() {
		sess.StartedAt = time.Now().UTC()
	}
	sess.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(sess.SessionID), data, 0o644)
}

// AppendTurn loads (or creates) the session and appends the given turn.
func (s *Store) AppendTurn(sessionID, agent string, t Turn) (*Session, error) {
	sess, err := s.Load(sessionID)
	if err != nil {
		return nil, err
	}
	if sess.Agent == "" {
		sess.Agent = agent
	}
	sess.Turns = append(sess.Turns, t)
	if err := s.Save(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// List returns all sessions, newest first by UpdatedAt.
func (s *Store) List() ([]*Session, error) {
	entries, err := os.ReadDir(s.repo.SessionsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		sess, err := s.Load(id)
		if err != nil {
			continue
		}
		out = append(out, sess)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// PromptSummary returns the first ~200 chars of the first user prompt in the session.
func (s *Session) PromptSummary() string {
	for _, t := range s.Turns {
		if t.UserPrompt != "" {
			p := strings.TrimSpace(t.UserPrompt)
			if len(p) > 200 {
				return p[:200] + "..."
			}
			return p
		}
	}
	return ""
}

// FindByCommit walks all sessions and returns the (session, turn) whose
// SnapshotHash equals the given commit hash, or nils if not found.
func (s *Store) FindByCommit(hash string) (*Session, *Turn, error) {
	sessions, err := s.List()
	if err != nil {
		return nil, nil, err
	}
	for _, sess := range sessions {
		for i := range sess.Turns {
			if sess.Turns[i].SnapshotHash == hash {
				return sess, &sess.Turns[i], nil
			}
		}
	}
	return nil, nil, nil
}
