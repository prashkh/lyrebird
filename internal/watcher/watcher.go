// Package watcher uses fsnotify to detect file changes in a tracked folder
// and triggers debounced snapshots.
package watcher

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/prashkh/lyrebird/internal/config"
	"github.com/prashkh/lyrebird/internal/gitstore"
)

// Watcher watches a single tracked folder for changes.
type Watcher struct {
	repo      *config.Repo
	store     *gitstore.Store
	debounce  time.Duration
	logger    *log.Logger

	mu          sync.Mutex
	pendingHits []string
	pendingTime time.Time
}

// New creates a watcher for the given repo. Debounce is the idle time the
// watcher waits after the last write before snapshotting.
func New(r *config.Repo, debounce time.Duration, logger *log.Logger) *Watcher {
	if logger == nil {
		logger = log.Default()
	}
	return &Watcher{
		repo:     r,
		store:    gitstore.New(r),
		debounce: debounce,
		logger:   logger,
	}
}

// shouldIgnore checks whether a path is one we never want to react to.
// (Real git-style ignore matching happens at commit time via core.excludesFile.)
func (w *Watcher) shouldIgnore(path string) bool {
	rel, err := filepath.Rel(w.repo.Root, path)
	if err != nil {
		return true
	}
	// Skip events inside our own .lyrebird/ — would cause infinite snapshot loops.
	if strings.HasPrefix(rel, config.DirName+string(filepath.Separator)) || rel == config.DirName {
		return true
	}
	// Skip common heavy directories at the root.
	heavyPrefixes := []string{
		".git/", ".git" + string(filepath.Separator),
		".venv/", ".venv" + string(filepath.Separator),
		"venv/",
		"node_modules/",
		"__pycache__/",
		".ipynb_checkpoints/",
		".DS_Store",
	}
	for _, p := range heavyPrefixes {
		if strings.HasPrefix(rel, p) || rel == strings.TrimSuffix(p, string(filepath.Separator)) {
			return true
		}
	}
	// Editor swap/temp files.
	base := filepath.Base(rel)
	if strings.HasPrefix(base, ".#") || strings.HasSuffix(base, "~") || strings.HasSuffix(base, ".swp") {
		return true
	}
	return false
}

// Run blocks until ctx is done (caller can stop via the watcher's underlying fsnotify Close).
// For simplicity v1 runs until process exit; we close the watcher then.
func (w *Watcher) Run() error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fw.Close()

	// Recursively add directories. fsnotify on macOS/Linux doesn't watch
	// subdirectories automatically, so we walk the tree at start and add
	// new dirs as they appear.
	if err := w.addRecursive(fw, w.repo.Root); err != nil {
		return err
	}

	w.logger.Printf("watcher started for %s (debounce=%v)", w.repo.Root, w.debounce)

	// Background goroutine that snapshots when the debounce timer fires.
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case ev, ok := <-fw.Events:
			if !ok {
				return nil
			}
			if w.shouldIgnore(ev.Name) {
				continue
			}
			// On Create of a directory, watch it.
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = fw.Add(ev.Name)
				}
			}
			w.mu.Lock()
			w.pendingHits = append(w.pendingHits, ev.Name)
			w.pendingTime = time.Now()
			w.mu.Unlock()
		case err, ok := <-fw.Errors:
			if !ok {
				return nil
			}
			w.logger.Printf("watcher error: %v", err)
		case <-tick.C:
			w.mu.Lock()
			pending := len(w.pendingHits)
			elapsed := time.Since(w.pendingTime)
			w.mu.Unlock()
			if pending > 0 && elapsed >= w.debounce {
				w.snapshot()
			}
		}
	}
}

// addRecursive walks root and adds every (non-ignored) directory to the watcher.
func (w *Watcher) addRecursive(fw *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}
		if !d.IsDir() {
			return nil
		}
		if w.shouldIgnore(path) {
			return filepath.SkipDir
		}
		return fw.Add(path)
	})
}

// snapshot drains pending hits and creates a single commit covering them.
func (w *Watcher) snapshot() {
	w.mu.Lock()
	hits := w.pendingHits
	w.pendingHits = nil
	w.mu.Unlock()
	if len(hits) == 0 {
		return
	}
	// Build a short summary of what changed.
	uniq := dedupRel(w.repo.Root, hits)
	limit := 5
	if len(uniq) > limit {
		uniq = append(uniq[:limit], fmt.Sprintf("... +%d more", len(hits)-limit))
	}
	msg := "[manual] " + strings.Join(uniq, " ")
	hash, err := w.store.Snapshot(msg)
	if err != nil {
		w.logger.Printf("snapshot failed: %v", err)
		return
	}
	if hash == "" {
		// All changes were inside ignored paths, nothing to commit.
		return
	}
	w.logger.Printf("snapshot %s: %s", hash[:12], msg)
}

func dedupRel(root string, paths []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			rel = p
		}
		if !seen[rel] {
			seen[rel] = true
			out = append(out, rel)
		}
	}
	return out
}
