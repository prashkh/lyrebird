// Package registry maintains ~/.lyre/registry.json — the central list of
// tracked folders, so a single `lyre ui` can show all projects regardless
// of where it's started from.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Project describes one tracked folder.
type Project struct {
	ID         string    `json:"id"`           // URL-safe slug derived from folder name
	Name       string    `json:"name"`         // human-friendly folder name
	Root       string    `json:"root"`         // absolute path to the tracked folder
	Registered time.Time `json:"registered"`   // when added to the registry
}

// Registry is the in-memory view of registry.json.
type Registry struct {
	Path     string    `json:"-"`
	Projects []Project `json:"projects"`
	mu       sync.Mutex
}

// DefaultPath returns ~/.lyre/registry.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".lyre", "registry.json"), nil
}

// Load reads the registry from path. A missing file returns an empty
// registry, not an error — so first-run is graceful.
func Load(path string) (*Registry, error) {
	r := &Registry{Path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	r.Path = path
	// Drop projects whose root no longer exists or no longer has a .lyrebird/.
	r.prune()
	return r, nil
}

// LoadDefault loads the registry from ~/.lyre/registry.json.
func LoadDefault() (*Registry, error) {
	p, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Load(p)
}

// Save persists the registry to disk.
func (r *Registry) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.Path, data, 0o644)
}

// Register adds (or updates) a project. Path is canonicalized.
func (r *Registry) Register(name, root string) (Project, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Project{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	// Already registered? Update name and slide to the front.
	for i, p := range r.Projects {
		if p.Root == abs {
			p.Name = name
			r.Projects = append([]Project{p}, append(r.Projects[:i], r.Projects[i+1:]...)...)
			return p, nil
		}
	}
	id := r.uniqueSlug(slugify(name))
	p := Project{ID: id, Name: name, Root: abs, Registered: time.Now().UTC()}
	r.Projects = append([]Project{p}, r.Projects...)
	return p, nil
}

// Unregister removes a project by ID.
func (r *Registry) Unregister(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, p := range r.Projects {
		if p.ID == id {
			r.Projects = append(r.Projects[:i], r.Projects[i+1:]...)
			return true
		}
	}
	return false
}

// ByID looks up a project. Returns nil if not found.
func (r *Registry) ByID(id string) *Project {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.Projects {
		if r.Projects[i].ID == id {
			return &r.Projects[i]
		}
	}
	return nil
}

// ByRoot looks up a project by its absolute folder path.
func (r *Registry) ByRoot(root string) *Project {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.Projects {
		if r.Projects[i].Root == abs {
			return &r.Projects[i]
		}
	}
	return nil
}

// All returns a defensive copy of the project list.
func (r *Registry) All() []Project {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Project, len(r.Projects))
	copy(out, r.Projects)
	return out
}

// prune drops projects whose root or .lyrebird directory has been deleted.
func (r *Registry) prune() {
	keep := r.Projects[:0]
	for _, p := range r.Projects {
		if _, err := os.Stat(filepath.Join(p.Root, ".lyrebird")); err == nil {
			keep = append(keep, p)
		}
	}
	r.Projects = keep
	sort.SliceStable(r.Projects, func(i, j int) bool {
		return r.Projects[i].Registered.After(r.Projects[j].Registered)
	})
}

// slugify produces a URL-safe lower-kebab-case slug from a name.
var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "project"
	}
	return s
}

// uniqueSlug ensures the slug doesn't collide with an existing project.
// Caller already holds r.mu.
func (r *Registry) uniqueSlug(base string) string {
	exists := func(s string) bool {
		for _, p := range r.Projects {
			if p.ID == s {
				return true
			}
		}
		return false
	}
	if !exists(base) {
		return base
	}
	for i := 2; i < 1000; i++ {
		c := fmt.Sprintf("%s-%d", base, i)
		if !exists(c) {
			return c
		}
	}
	return base + "-x"
}
