// Package config handles per-folder Lyrebird configuration and repo discovery.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// DirName is the hidden directory inside a tracked folder.
	DirName = ".lyrebird"
	// ConfigFile lives inside DirName.
	ConfigFile = "config.json"
	// GitDir holds the embedded git repo.
	GitDir = "git"
	// SessionsDir holds per-AI-session JSON files.
	SessionsDir = "sessions"
	// HandoffsDir holds generated handoff packages.
	HandoffsDir = "handoffs"
	// IgnoreFile is at the repo root.
	IgnoreFile = ".lyreignore"
)

// Config is the per-folder configuration stored at .lyrebird/config.json.
type Config struct {
	Version    int    `json:"version"`
	Created    string `json:"created"`
	FolderName string `json:"folder_name"`
}

// Repo represents a tracked folder.
type Repo struct {
	Root       string // absolute path to the tracked folder root
	LyrebirdDir string // <Root>/.lyrebird
	GitDir     string // <Root>/.lyrebird/git
	SessionsDir string // <Root>/.lyrebird/sessions
	HandoffsDir string // <Root>/.lyrebird/handoffs
	Config     Config
}

// FindRoot walks up from start until it finds a .lyrebird/ directory.
// Returns the path that contains .lyrebird/, not .lyrebird itself.
func FindRoot(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	cur := abs
	for {
		candidate := filepath.Join(cur, DirName)
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("not in a lyrebird repo (no %s found in %s or any parent)", DirName, abs)
		}
		cur = parent
	}
}

// Open opens an existing repo found by walking up from start.
func Open(start string) (*Repo, error) {
	root, err := FindRoot(start)
	if err != nil {
		return nil, err
	}
	return openAt(root)
}

func openAt(root string) (*Repo, error) {
	r := &Repo{
		Root:        root,
		LyrebirdDir: filepath.Join(root, DirName),
		GitDir:      filepath.Join(root, DirName, GitDir),
		SessionsDir: filepath.Join(root, DirName, SessionsDir),
		HandoffsDir: filepath.Join(root, DirName, HandoffsDir),
	}
	cfgPath := filepath.Join(r.LyrebirdDir, ConfigFile)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", cfgPath, err)
	}
	if err := json.Unmarshal(data, &r.Config); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", cfgPath, err)
	}
	return r, nil
}

// CreateAt initializes a new lyrebird repo at root. Errors if one already exists.
func CreateAt(root string, cfg Config) (*Repo, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(abs, DirName)); err == nil {
		return nil, errors.New("already initialized: " + filepath.Join(abs, DirName))
	}
	for _, d := range []string{
		filepath.Join(abs, DirName),
		filepath.Join(abs, DirName, GitDir),
		filepath.Join(abs, DirName, SessionsDir),
		filepath.Join(abs, DirName, HandoffsDir),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	cfgPath := filepath.Join(abs, DirName, ConfigFile)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		return nil, err
	}
	return openAt(abs)
}

// Save persists the config back to disk.
func (r *Repo) Save() error {
	cfgPath := filepath.Join(r.LyrebirdDir, ConfigFile)
	data, err := json.MarshalIndent(r.Config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0o644)
}

// IgnorePath returns the absolute path to .lyreignore at the repo root.
func (r *Repo) IgnorePath() string {
	return filepath.Join(r.Root, IgnoreFile)
}

// DefaultIgnoreContents are written to .lyreignore on init.
const DefaultIgnoreContents = `# Lyrebird ignore patterns. Same syntax as .gitignore.
# Files matched here are NEVER snapshotted.

# Python
.venv/
venv/
__pycache__/
*.pyc
*.pyo
.ipynb_checkpoints/

# Node
node_modules/

# OS / editor cruft
.DS_Store
*.swp
.idea/
.vscode/

# Lyrebird itself
.lyrebird/

# Note: large binaries (.hdf5, .gds, .png) are NOT ignored by default.
# Lyrebird tracks them so you can restore. Add patterns here to opt out.
`
