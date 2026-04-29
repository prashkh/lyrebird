// Package gitstore wraps the system `git` binary so Lyrebird stores its
// snapshots in a hidden git repo at .lyrebird/git/, with the user's folder
// as the work tree. The user's own .git/ (if any) is invisible to us.
package gitstore

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/prashkh/lyrebird/internal/config"
)

// Store is a thin wrapper around git invocations targeted at our hidden repo.
type Store struct {
	repo *config.Repo
}

func New(r *config.Repo) *Store {
	return &Store{repo: r}
}

// env returns environment variables that pin git to our hidden repo.
func (s *Store) env() []string {
	env := os.Environ()
	env = append(env,
		"GIT_DIR="+s.repo.GitDir,
		"GIT_WORK_TREE="+s.repo.Root,
	)
	return env
}

// run executes git with the given args and returns stdout.
func (s *Store) run(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Env = s.env()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// Init creates the hidden git repo (idempotent).
func (s *Store) Init() error {
	// `git init` creates the dir if missing. We use --quiet for silent operation.
	if _, err := s.run("init", "--quiet", "-b", "lyre-snapshots", s.repo.GitDir); err != nil {
		return err
	}
	// Configure a safe identity so commits don't fail when user has no git config.
	if _, err := s.run("config", "user.name", "Lyrebird"); err != nil {
		return err
	}
	if _, err := s.run("config", "user.email", "lyrebird@localhost"); err != nil {
		return err
	}
	// Speed up status on big trees.
	_, _ = s.run("config", "core.untrackedCache", "true")
	// Use the repo root's .lyreignore as the exclude file (in addition to .gitignore).
	_, _ = s.run("config", "core.excludesFile", filepath.Join(s.repo.Root, config.IgnoreFile))
	return nil
}

// Snapshot stages everything (respecting .lyreignore) and creates a commit.
// Returns the new commit hash. If nothing changed, returns "" without error.
func (s *Store) Snapshot(message string) (string, error) {
	// `git add -A` stages all changes including deletions, respecting excludes.
	if _, err := s.run("add", "-A"); err != nil {
		return "", err
	}
	// Detect whether there's anything staged. `git diff --cached --quiet` exits 0 if no diff.
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Env = s.env()
	if err := cmd.Run(); err == nil {
		// Nothing staged. Check if this is the very first commit (empty repo).
		if _, headErr := s.run("rev-parse", "--verify", "HEAD"); headErr == nil {
			return "", nil
		}
	}
	// Commit. --allow-empty for the first commit when only ignored files exist;
	// we explicitly bail above otherwise.
	if _, err := s.run("commit", "--allow-empty-message", "--allow-empty", "-q", "-m", message); err != nil {
		return "", err
	}
	out, err := s.run("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Log entry parsed from a custom git log format.
type LogEntry struct {
	Hash      string
	ShortHash string
	Author    string
	Date      string // ISO8601
	Subject   string
	Body      string
}

// Log returns the most recent N log entries (N=0 means all).
func (s *Store) Log(n int, paths ...string) ([]LogEntry, error) {
	// Use NUL bytes as field/record separators so commit messages with newlines don't break parsing.
	const fieldSep = "\x1f" // unit separator
	const recordSep = "\x1e" // record separator
	format := "%H" + fieldSep + "%h" + fieldSep + "%an" + fieldSep + "%aI" + fieldSep + "%s" + fieldSep + "%b" + recordSep
	args := []string{"log", "--format=" + format}
	if n > 0 {
		args = append(args, fmt.Sprintf("-n%d", n))
	}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	out, err := s.run(args...)
	if err != nil {
		// Empty repo with no commits: return empty list, not error.
		if strings.Contains(err.Error(), "does not have any commits yet") ||
			strings.Contains(err.Error(), "bad default revision") ||
			strings.Contains(err.Error(), "unknown revision") {
			return nil, nil
		}
		return nil, err
	}
	var entries []LogEntry
	records := strings.Split(string(out), recordSep)
	for _, rec := range records {
		rec = strings.TrimLeft(rec, "\n")
		if rec == "" {
			continue
		}
		parts := strings.SplitN(rec, fieldSep, 6)
		if len(parts) < 6 {
			continue
		}
		entries = append(entries, LogEntry{
			Hash:      parts[0],
			ShortHash: parts[1],
			Author:    parts[2],
			Date:      parts[3],
			Subject:   parts[4],
			Body:      strings.TrimSpace(parts[5]),
		})
	}
	return entries, nil
}

// Show returns the full commit message + diff for a hash.
func (s *Store) Show(hash string) (string, error) {
	out, err := s.run("show", "--patch", "--stat", hash)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Diff between two refs (or a ref and the working tree if b is empty).
func (s *Store) Diff(a, b string, paths ...string) (string, error) {
	args := []string{"diff", a}
	if b != "" {
		args = append(args, b)
	}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	out, err := s.run(args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// FilesChanged returns the list of files modified in commit `hash`.
func (s *Store) FilesChanged(hash string) ([]string, error) {
	out, err := s.run("show", "--name-only", "--format=", hash)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// Restore overwrites a single file in the work tree with its content from `hash`.
func (s *Store) Restore(hash, path string) error {
	_, err := s.run("checkout", hash, "--", path)
	return err
}

// Revert resets the entire work tree to the state at `hash`.
// Caller is responsible for taking a safety snapshot first.
func (s *Store) Revert(hash string) error {
	// Use checkout-index style: reset working tree to commit, but keep our HEAD on the snapshots branch.
	if _, err := s.run("read-tree", "-u", "--reset", hash); err != nil {
		return err
	}
	return nil
}

// FileAtRef returns file contents at a given ref. Returns nil bytes if file didn't exist.
func (s *Store) FileAtRef(ref, path string) ([]byte, error) {
	out, err := s.run("show", ref+":"+path)
	if err != nil {
		// File didn't exist at that ref.
		if strings.Contains(err.Error(), "exists on disk, but not in") ||
			strings.Contains(err.Error(), "does not exist") {
			return nil, nil
		}
		return nil, err
	}
	return out, nil
}

// HasCommits reports whether the repo has at least one commit.
func (s *Store) HasCommits() bool {
	_, err := s.run("rev-parse", "--verify", "HEAD")
	return err == nil
}

// LsFiles returns the list of files tracked at HEAD (paths relative to repo root).
func (s *Store) LsFiles() ([]string, error) {
	out, err := s.run("ls-tree", "-r", "--name-only", "HEAD")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// CurrentHead returns the current HEAD hash, or "" if no commits yet.
func (s *Store) CurrentHead() string {
	out, err := s.run("rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
