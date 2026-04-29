// Package ui serves the local Lyrebird web UI.
package ui

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/prashkh/lyrebird/internal/config"
	"github.com/prashkh/lyrebird/internal/gitstore"
	"github.com/prashkh/lyrebird/internal/handoff"
	"github.com/prashkh/lyrebird/internal/session"
)

//go:embed templates/*.html
var tmplFS embed.FS

// Server holds dependencies needed to render the UI.
type Server struct {
	repo  *config.Repo
	store *gitstore.Store
	sess  *session.Store
	funcs template.FuncMap
}

// New constructs the server.
func New(r *config.Repo) (*Server, error) {
	s := &Server{
		repo:  r,
		store: gitstore.New(r),
		sess:  session.New(r),
	}
	funcs := template.FuncMap{
		"add":       func(a, b int) int { return a + b },
		"shortHash": func(h string) string {
			if len(h) > 12 {
				return h[:12]
			}
			return h
		},
		"shortID": func(id string) string {
			if len(id) > 12 {
				return id[:12] + "…"
			}
			return id
		},
		"truncate": func(n int, s string) string {
			s = strings.ReplaceAll(s, "\n", " ")
			if len(s) > n {
				return s[:n] + "…"
			}
			return s
		},
	}
	s.funcs = funcs
	// Verify all templates parse at startup so we fail fast.
	if _, err := template.New("").Funcs(funcs).ParseFS(tmplFS, "templates/*.html"); err != nil {
		return nil, err
	}
	return s, nil
}

// Routes registers HTTP handlers on a fresh ServeMux and returns it.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleTimeline)
	mux.HandleFunc("/sessions", s.handleSessions)
	mux.HandleFunc("/sessions/", s.handleSession)
	mux.HandleFunc("/show/", s.handleShow)
	mux.HandleFunc("/file", s.handleFile)
	mux.HandleFunc("/search", s.handleSearch)
	mux.HandleFunc("/handoff", s.handleHandoff)
	mux.HandleFunc("/restore", s.handleRestore)
	mux.HandleFunc("/undo", s.handleUndo)
	mux.HandleFunc("/snapshot", s.handleSnapshot)
	return mux
}

// handleUndo rolls the folder back to the state BEFORE the most recent
// non-system change. Always reversible — takes a safety checkpoint first.
func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	entries, err := s.store.Log(0)
	if err != nil {
		httpErr(w, err)
		return
	}
	// Find the most recent non-system event, then the snapshot just before it.
	var undoFromIdx = -1
	for i, e := range entries {
		if !strings.HasPrefix(e.Subject, "[lyre]") &&
			!strings.HasPrefix(e.Subject, "[safety]") &&
			!strings.HasPrefix(e.Subject, "[restore]") &&
			!strings.HasPrefix(e.Subject, "[revert]") {
			undoFromIdx = i
			break
		}
	}
	if undoFromIdx == -1 {
		http.Redirect(w, r, "/?flash="+template.URLQueryEscaper("Nothing to undo."), http.StatusFound)
		return
	}
	// Target = snapshot one position older. If the change is the very first
	// real change, fall back to the first commit (lyre init).
	target := ""
	if undoFromIdx+1 < len(entries) {
		target = entries[undoFromIdx+1].Hash
	} else {
		http.Redirect(w, r, "/?flash="+template.URLQueryEscaper("This is the first change — nothing to undo to."), http.StatusFound)
		return
	}
	if _, err := s.store.Snapshot("[safety] before undo"); err != nil {
		httpErr(w, err)
		return
	}
	if err := s.store.Revert(target); err != nil {
		httpErr(w, err)
		return
	}
	_, _ = s.store.Snapshot("[revert] undid most recent change")
	http.Redirect(w, r, "/?flash="+template.URLQueryEscaper("Undone. Click 'Undo last change' again to undo this undo."), http.StatusFound)
}

// handleSnapshot creates a manual snapshot with an optional note.
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		httpErr(w, err)
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	msg := "[manual] manual save"
	if note != "" {
		msg = "[manual] " + note
	}
	hash, err := s.store.Snapshot(msg)
	if err != nil {
		httpErr(w, err)
		return
	}
	flash := "Saved a snapshot."
	if hash == "" {
		flash = "Nothing changed since the last save."
	}
	http.Redirect(w, r, "/?flash="+template.URLQueryEscaper(flash), http.StatusFound)
}

type eventVM struct {
	Hash        string
	ShortHash   string
	Date        string // human-friendly: "just now", "5 min ago", "Today at 3:42 pm"
	When        time.Time
	Subject     string
	Headline    string // friendlier rendering of the change
	Files       []string
	Actor       string // "You", "Claude", "Lyrebird"
	ActorIcon   string // "✋", "🤖", "🐦"
	ActorClass  string // "you", "ai", "lyre"
	IsAI        bool
	IsLyre      bool   // true for [lyre]/[safety]/[restore]/[revert]
	IsQuiet     bool   // true for events with no useful diff to show ([lyre], [safety])
	Agent       string
	SessionID   string
	UserPrompt  string
	DayBucket   string // "Today", "Yesterday", "Tuesday", "Apr 12"
}

// summaryVM is the "what's been happening here" panel at the top of the timeline.
type summaryVM struct {
	FolderName     string
	TrackedSince   string
	TotalSnapshots int
	AISnapshots    int
	ManualSnapshots int
	SessionsCount  int
	FilesCount     int
	FileList       []string  // up to ~12 names
	MoreFiles      int
	LastActivity   string    // human-friendly
	LastAISession  *session.Session
	HasHook        bool
}

func (s *Server) buildEvents(n int) ([]eventVM, error) {
	entries, err := s.store.Log(n)
	if err != nil {
		return nil, err
	}
	var out []eventVM
	for _, e := range entries {
		ev := eventVM{
			Hash:      e.Hash,
			ShortHash: e.ShortHash,
			Subject:   e.Subject,
		}
		if t, err := time.Parse(time.RFC3339, e.Date); err == nil {
			ev.When = t
			ev.Date = humanizeAgo(t)
			ev.DayBucket = dayBucket(t)
		} else {
			ev.Date = e.Date
		}
		files, err := s.store.FilesChanged(e.Hash)
		if err == nil && len(files) > 0 {
			files = filterDisplayFiles(files)
			limit := 6
			if len(files) > limit {
				ev.Files = append(files[:limit], fmt.Sprintf("…+%d more", len(files)-limit))
			} else {
				ev.Files = files
			}
		}
		// Decide actor/headline based on subject prefix.
		switch {
		case strings.HasPrefix(e.Subject, "[ai]"):
			ev.IsAI = true
		case strings.HasPrefix(e.Subject, "[lyre]"),
			strings.HasPrefix(e.Subject, "[safety]"):
			ev.IsLyre = true
			ev.IsQuiet = true
		case strings.HasPrefix(e.Subject, "[restore]"),
			strings.HasPrefix(e.Subject, "[revert]"):
			ev.IsLyre = true
		}
		if sess, turn, _ := s.sess.FindByCommit(e.Hash); sess != nil && turn != nil {
			ev.IsAI = true
			ev.IsLyre = false
			ev.Agent = sess.Agent
			ev.SessionID = sess.SessionID
			p := turn.UserPrompt
			if len(p) > 240 {
				p = p[:240] + "…"
			}
			ev.UserPrompt = p
		}
		ev.Actor, ev.ActorIcon, ev.ActorClass = actorFor(ev)
		ev.Headline = renderHeadline(e.Subject, ev.Files, ev.Actor)
		out = append(out, ev)
	}
	return out, nil
}

// actorFor returns the friendly name, icon, and CSS class for an event.
func actorFor(ev eventVM) (name, icon, class string) {
	switch {
	case ev.IsAI:
		switch ev.Agent {
		case "claude-code":
			return "Claude", "🤖", "ai"
		case "":
			return "AI", "🤖", "ai"
		default:
			return ev.Agent, "🤖", "ai"
		}
	case ev.IsLyre:
		return "Lyrebird", "🐦", "lyre"
	default:
		return "You", "✋", "you"
	}
}

// dayBucket returns "Today", "Yesterday", weekday name (within 7 days), or "Apr 27".
func dayBucket(t time.Time) string {
	now := time.Now()
	tmid := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	nowmid := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	days := int(nowmid.Sub(tmid).Hours() / 24)
	switch {
	case days == 0:
		return "Today"
	case days == 1:
		return "Yesterday"
	case days < 7:
		return t.Format("Monday")
	default:
		return t.Format("Jan 2")
	}
}

// groupEventsByDay turns a flat newest-first event list into day-bucketed groups.
func groupEventsByDay(events []eventVM) []dayGroupVM {
	var groups []dayGroupVM
	for _, ev := range events {
		bucket := ev.DayBucket
		if bucket == "" {
			bucket = "Earlier"
		}
		if len(groups) == 0 || groups[len(groups)-1].Label != bucket {
			groups = append(groups, dayGroupVM{Label: bucket})
		}
		groups[len(groups)-1].Events = append(groups[len(groups)-1].Events, ev)
	}
	return groups
}

// dayGroupVM groups events under a day header for the story view.
type dayGroupVM struct {
	Label  string
	Events []eventVM
}

// filterDisplayFiles drops paths that are obvious editor artifacts or
// Lyrebird-internal config files from the list shown in the UI.
func filterDisplayFiles(files []string) []string {
	var out []string
	for _, f := range files {
		base := f
		if i := strings.LastIndex(f, "/"); i >= 0 {
			base = f[i+1:]
		}
		if isUITempArtifact(base) {
			continue
		}
		// Lyrebird's own config — internal, don't bother the user.
		if f == ".lyreignore" {
			continue
		}
		out = append(out, f)
	}
	return out
}

func isUITempArtifact(base string) bool {
	if base == ".DS_Store" {
		return true
	}
	if strings.HasSuffix(base, ".tmp") || strings.HasSuffix(base, "~") {
		return true
	}
	if strings.HasPrefix(base, ".#") {
		return true
	}
	if i := strings.Index(base, ".tmp."); i >= 0 {
		tail := base[i+5:]
		ok := tail != ""
		for _, r := range tail {
			if (r < '0' || r > '9') && r != '.' {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	for _, ext := range []string{".swp", ".swo", ".swn", ".bak", ".orig", ".rej", ".pyc", ".pyo"} {
		if strings.HasSuffix(base, ext) {
			return true
		}
	}
	return false
}

// renderHeadline turns a raw commit subject into a friendly story sentence.
// Examples (actor = "You" / "Claude" / "Lyrebird"):
//   [manual] hello.py notes.md       → "Edited hello.py and notes.md"
//   [ai] claude-code sess_abc fib.py → "Edited fib.py"
//   [lyre] initial ...               → "Started tracking this folder"
//   [restore] foo.py from abc123     → "Brought foo.py back to an earlier version"
//   [safety] ...                     → "Saved a checkpoint"
func renderHeadline(subject string, files []string, actor string) string {
	switch {
	case strings.HasPrefix(subject, "[lyre]"):
		return "Started tracking this folder"
	case strings.HasPrefix(subject, "[safety]"):
		return "Saved a checkpoint before undoing"
	case strings.HasPrefix(subject, "[restore]"):
		// "fib.py from 0db834a" → "Brought fib.py back to an earlier version"
		rest := strings.TrimSpace(strings.TrimPrefix(subject, "[restore]"))
		fileName := rest
		if i := strings.Index(rest, " from "); i >= 0 {
			fileName = rest[:i]
		}
		return "Brought " + fileName + " back to an earlier version"
	case strings.HasPrefix(subject, "[revert]"):
		return "Rolled the folder back to an earlier state"
	}
	if len(files) == 0 {
		return "Made a change"
	}
	verb := "Edited"
	if actor == "Claude" || actor == "AI" {
		verb = "Edited"
	}
	return verb + " " + humanList(files, 3)
}

// humanList formats a file list with proper "and" grammar.
//   ["a"]            → "a"
//   ["a","b"]        → "a and b"
//   ["a","b","c"]    → "a, b, and c"
//   ["a","b","c","d"] with max 3 → "a, b, c, and 1 more"
func humanList(s []string, max int) string {
	if len(s) == 0 {
		return ""
	}
	if len(s) == 1 {
		return s[0]
	}
	more := 0
	if len(s) > max {
		more = len(s) - max
		s = s[:max]
	}
	if len(s) == 2 && more == 0 {
		return s[0] + " and " + s[1]
	}
	suffix := ""
	if more > 0 {
		suffix = fmt.Sprintf(", and %d more", more)
	} else {
		// last item gets "and" prefix
		last := s[len(s)-1]
		s = s[:len(s)-1]
		suffix = ", and " + last
	}
	return strings.Join(s, ", ") + suffix
}

// buildSummary produces the data shown in the timeline's "what's happening here" panel.
func (s *Server) buildSummary() (*summaryVM, error) {
	entries, err := s.store.Log(0)
	if err != nil {
		return nil, err
	}
	sessions, _ := s.sess.List()
	sm := &summaryVM{
		FolderName:     s.repo.Config.FolderName,
		TotalSnapshots: len(entries),
		SessionsCount:  len(sessions),
	}
	if t, err := time.Parse(time.RFC3339, s.repo.Config.Created); err == nil {
		sm.TrackedSince = t.Format("Jan 2, 2006")
	} else {
		sm.TrackedSince = s.repo.Config.Created
	}
	for _, e := range entries {
		if sess, _, _ := s.sess.FindByCommit(e.Hash); sess != nil {
			sm.AISnapshots++
		} else {
			sm.ManualSnapshots++
		}
	}
	if len(entries) > 0 {
		if t, err := time.Parse(time.RFC3339, entries[0].Date); err == nil {
			sm.LastActivity = humanizeAgo(t)
		}
	}
	if len(sessions) > 0 {
		sm.LastAISession = sessions[0]
	}
	// Files in current state via `git ls-tree`.
	if files, err := s.listFiles(); err == nil {
		filtered := filterDisplayFiles(files)
		sm.FilesCount = len(filtered)
		max := 12
		if len(filtered) > max {
			sm.FileList = filtered[:max]
			sm.MoreFiles = len(filtered) - max
		} else {
			sm.FileList = filtered
		}
	}
	// Detect whether the Claude Code hook is installed.
	sm.HasHook = isHookInstalled()
	return sm, nil
}

// listFiles returns paths tracked at HEAD.
func (s *Server) listFiles() ([]string, error) {
	if !s.store.HasCommits() {
		return nil, nil
	}
	out, err := s.store.LsFiles()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func humanizeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
	return t.Format("Jan 2, 2006")
}

// isHookInstalled checks whether ~/.claude/settings.json mentions our hook.
func isHookInstalled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(home + "/.claude/settings.json")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "lyre hook") || strings.Contains(string(data), "/lyre")
}

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	events, err := s.buildEvents(100)
	if err != nil {
		httpErr(w, err)
		return
	}
	summary, err := s.buildSummary()
	if err != nil {
		httpErr(w, err)
		return
	}
	flash := r.URL.Query().Get("flash")
	groups := groupEventsByDay(events)
	// Determine the most-recent non-system event (for "Undo last change").
	var lastUndoableShort, lastUndoableHeadline string
	for _, ev := range events {
		if !ev.IsLyre {
			lastUndoableShort = ev.Hash
			lastUndoableHeadline = ev.Headline
			break
		}
	}
	s.render(w, "timeline.html", map[string]any{
		"Title":              "Timeline",
		"Repo":               s.repo,
		"Events":             events,
		"DayGroups":          groups,
		"Summary":            summary,
		"Flash":              flash,
		"LastUndoableHash":   lastUndoableShort,
		"LastUndoableHeadline": lastUndoableHeadline,
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	list, err := s.sess.List()
	if err != nil {
		httpErr(w, err)
		return
	}
	s.render(w, "sessions.html", map[string]any{
		"Title":    "Sessions",
		"Sessions": list,
	})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if id == "" {
		http.Redirect(w, r, "/sessions", http.StatusFound)
		return
	}
	list, err := s.sess.List()
	if err != nil {
		httpErr(w, err)
		return
	}
	var match *session.Session
	for _, sess := range list {
		if strings.HasPrefix(sess.SessionID, id) {
			match = sess
			break
		}
	}
	if match == nil {
		http.NotFound(w, r)
		return
	}
	// Sort turns chronologically.
	turns := append([]session.Turn(nil), match.Turns...)
	sort.SliceStable(turns, func(i, j int) bool {
		return turns[i].Timestamp.Before(turns[j].Timestamp)
	})
	view := *match
	view.Turns = turns
	s.render(w, "session.html", map[string]any{
		"Title":   "Session " + id,
		"Session": &view,
	})
}

func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
	hash := strings.TrimPrefix(r.URL.Path, "/show/")
	if hash == "" {
		http.NotFound(w, r)
		return
	}
	entries, err := s.store.Log(0)
	if err != nil {
		httpErr(w, err)
		return
	}
	var match *gitstore.LogEntry
	for i := range entries {
		if strings.HasPrefix(entries[i].Hash, hash) {
			match = &entries[i]
			break
		}
	}
	if match == nil {
		http.NotFound(w, r)
		return
	}
	patch, _ := s.store.ShowPatch(match.Hash)
	stat, _ := s.store.ShowStat(match.Hash)
	groups := parseDiff(patch)
	sess, turn, _ := s.sess.FindByCommit(match.Hash)

	// Build a small eventVM-like view so we can re-use actor + headline logic.
	files, _ := s.store.FilesChanged(match.Hash)
	files = filterDisplayFiles(files)
	ev := eventVM{Subject: match.Subject}
	switch {
	case strings.HasPrefix(match.Subject, "[ai]"):
		ev.IsAI = true
	case strings.HasPrefix(match.Subject, "[lyre]"),
		strings.HasPrefix(match.Subject, "[safety]"),
		strings.HasPrefix(match.Subject, "[restore]"),
		strings.HasPrefix(match.Subject, "[revert]"):
		ev.IsLyre = true
	}
	if sess != nil {
		ev.IsAI = true
		ev.IsLyre = false
		ev.Agent = sess.Agent
	}
	ev.Actor, ev.ActorIcon, ev.ActorClass = actorFor(ev)
	headline := renderHeadline(match.Subject, files, ev.Actor)

	date := match.Date
	if t, err := time.Parse(time.RFC3339, match.Date); err == nil {
		date = humanizeAgo(t) + " (" + t.Format("Jan 2 · 3:04 pm") + ")"
	}
	s.render(w, "show.html", map[string]any{
		"Title":      "What changed",
		"Hash":       match.Hash,
		"ShortHash":  match.ShortHash,
		"Date":       date,
		"Subject":    match.Subject,
		"Headline":   headline,
		"Actor":      ev.Actor,
		"DiffGroups": groups,
		"DiffStat":   strings.TrimSpace(stat),
		"Session":    sess,
		"Turn":       turn,
	})
}

// DiffLine is one line of a parsed unified diff with a class hint.
type DiffLine struct {
	Class string // "add", "del", "hunk", "context"
	Text  string
}

// DiffFile groups diff lines by file. The "Header" is the human-readable
// path (e.g. "fib.py") plus an indicator (new file, deleted file, renamed).
type DiffFile struct {
	Path  string
	Note  string     // "new file", "deleted file", "renamed", or ""
	Lines []DiffLine
}

// parseDiff splits a unified diff (patch only — no commit header) into per-file
// groups with classified lines. Lines starting with `diff --git`/`index`/`+++ b/`
// `--- a/` get folded into the file header; `@@` becomes a hunk separator.
func parseDiff(patch string) []DiffFile {
	var out []DiffFile
	var cur *DiffFile
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			// "diff --git a/foo b/foo" → "foo"
			parts := strings.Fields(line)
			path := ""
			if len(parts) >= 4 {
				p := parts[3] // "b/foo"
				path = strings.TrimPrefix(p, "b/")
			}
			cur = &DiffFile{Path: path}
		case cur != nil && strings.HasPrefix(line, "new file"):
			cur.Note = "new file"
		case cur != nil && strings.HasPrefix(line, "deleted file"):
			cur.Note = "deleted file"
		case cur != nil && strings.HasPrefix(line, "rename "):
			cur.Note = "renamed"
		case cur != nil && (strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "--- ") ||
			strings.HasPrefix(line, "+++ ") ||
			strings.HasPrefix(line, "old mode") ||
			strings.HasPrefix(line, "new mode") ||
			strings.HasPrefix(line, "similarity ") ||
			strings.HasPrefix(line, "Binary files ")):
			// File-level metadata — skip in the per-file render. We already show Path/Note.
			if strings.HasPrefix(line, "Binary files ") && cur != nil {
				cur.Note = "binary"
			}
		case cur != nil && strings.HasPrefix(line, "@@"):
			cur.Lines = append(cur.Lines, DiffLine{Class: "hunk", Text: line})
		case cur != nil && strings.HasPrefix(line, "+"):
			cur.Lines = append(cur.Lines, DiffLine{Class: "add", Text: strings.TrimPrefix(line, "+")})
		case cur != nil && strings.HasPrefix(line, "-"):
			cur.Lines = append(cur.Lines, DiffLine{Class: "del", Text: strings.TrimPrefix(line, "-")})
		case cur != nil:
			// context line (starts with space or empty).
			if strings.HasPrefix(line, " ") {
				line = strings.TrimPrefix(line, " ")
			}
			cur.Lines = append(cur.Lines, DiffLine{Class: "context", Text: line})
		}
	}
	flush()
	return out
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	entries, err := s.store.Log(0, path)
	if err != nil {
		httpErr(w, err)
		return
	}
	type version struct {
		Hash       string
		ShortHash  string
		Date       string
		Subject    string
		Headline   string
		Actor      string
		ActorIcon  string
		ActorClass string
		UserPrompt string
	}
	var versions []version
	for _, e := range entries {
		v := version{Hash: e.Hash, ShortHash: e.ShortHash, Subject: e.Subject}
		if t, err := time.Parse(time.RFC3339, e.Date); err == nil {
			v.Date = humanizeAgo(t)
		} else {
			v.Date = e.Date
		}
		ev := eventVM{Subject: e.Subject}
		switch {
		case strings.HasPrefix(e.Subject, "[ai]"):
			ev.IsAI = true
		case strings.HasPrefix(e.Subject, "[lyre]"),
			strings.HasPrefix(e.Subject, "[safety]"),
			strings.HasPrefix(e.Subject, "[restore]"),
			strings.HasPrefix(e.Subject, "[revert]"):
			ev.IsLyre = true
		}
		if sess, turn, _ := s.sess.FindByCommit(e.Hash); sess != nil && turn != nil {
			ev.IsAI = true
			ev.IsLyre = false
			ev.Agent = sess.Agent
			p := turn.UserPrompt
			if len(p) > 200 {
				p = p[:200] + "…"
			}
			v.UserPrompt = p
		}
		v.Actor, v.ActorIcon, v.ActorClass = actorFor(ev)
		v.Headline = renderHeadline(e.Subject, []string{path}, v.Actor)
		versions = append(versions, v)
	}
	s.render(w, "file.html", map[string]any{
		"Title":    path,
		"Path":     path,
		"Versions": versions,
	})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	type result struct {
		Hash      string
		ShortHash string
		Date      string
		Subject   string
		IsAI      bool
		Agent     string
		Highlight template.HTML
	}
	var results []result
	if q != "" {
		entries, err := s.store.Log(0)
		if err != nil {
			httpErr(w, err)
			return
		}
		ql := strings.ToLower(q)
		for _, e := range entries {
			subjLower := strings.ToLower(e.Subject)
			bodyLower := strings.ToLower(e.Body)
			matched := strings.Contains(subjLower, ql) || strings.Contains(bodyLower, ql)
			snippet := ""
			if !matched {
				if _, turn, _ := s.sess.FindByCommit(e.Hash); turn != nil {
					full := turn.UserPrompt + "\n" + turn.AssistantText
					if strings.Contains(strings.ToLower(full), ql) {
						matched = true
						snippet = makeSnippet(full, q)
					}
				}
			}
			if matched {
				if snippet == "" && strings.Contains(bodyLower, ql) {
					snippet = makeSnippet(e.Body, q)
				}
				if snippet == "" {
					snippet = e.Subject
				}
				date := e.Date
				if t, err := time.Parse(time.RFC3339, e.Date); err == nil {
					date = t.Format("2006-01-02 15:04")
				}
				res := result{Hash: e.Hash, ShortHash: e.ShortHash, Date: date, Subject: e.Subject}
				if sess, _, _ := s.sess.FindByCommit(e.Hash); sess != nil {
					res.IsAI = true
					res.Agent = sess.Agent
				}
				res.Highlight = template.HTML(highlight(snippet, q))
				results = append(results, res)
			}
		}
	}
	s.render(w, "search.html", map[string]any{
		"Title":   "Search",
		"Query":   q,
		"Results": results,
	})
}

func (s *Server) handleHandoff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	dir, err := handoff.Package(s.repo, "")
	if err != nil {
		httpErr(w, err)
		return
	}
	// Read the HANDOFF.md back for preview.
	mdBytes, _ := os.ReadFile(dir + "/HANDOFF.md")
	s.render(w, "handoff.html", map[string]any{
		"Title":     "Handoff",
		"Path":      dir,
		"HandoffMD": string(mdBytes),
	})
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		httpErr(w, err)
		return
	}
	path := r.FormValue("path")
	hash := r.FormValue("hash")
	if path == "" || hash == "" {
		http.Error(w, "missing path or hash", http.StatusBadRequest)
		return
	}
	if _, err := s.store.Snapshot("[safety] before restore of " + path + " from " + hash); err != nil {
		httpErr(w, err)
		return
	}
	if err := s.store.Restore(hash, path); err != nil {
		httpErr(w, err)
		return
	}
	_, _ = s.store.Snapshot("[restore] " + path + " from " + hash)
	http.Redirect(w, r, "/?flash="+template.URLQueryEscaper(fmt.Sprintf("Restored %s to %s", path, hash[:8])), http.StatusFound)
}

func (s *Server) render(w http.ResponseWriter, page string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["Title"]; !ok {
		data["Title"] = "Lyrebird"
	}
	if _, ok := data["Query"]; !ok {
		data["Query"] = ""
	}
	// We parse the named page template plus _layout, then execute "layout".
	tmpl, err := template.New("").Funcs(s.funcs).ParseFS(tmplFS, "templates/_layout.html", "templates/"+page)
	if err != nil {
		httpErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		httpErr(w, err)
	}
}

func httpErr(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// makeSnippet returns a window of text around the first match of q.
func makeSnippet(text, q string) string {
	ql := strings.ToLower(q)
	tl := strings.ToLower(text)
	idx := strings.Index(tl, ql)
	if idx == -1 {
		if len(text) > 200 {
			return text[:200] + "…"
		}
		return text
	}
	start := idx - 60
	if start < 0 {
		start = 0
	}
	end := idx + len(q) + 60
	if end > len(text) {
		end = len(text)
	}
	prefix := ""
	suffix := ""
	if start > 0 {
		prefix = "…"
	}
	if end < len(text) {
		suffix = "…"
	}
	return prefix + text[start:end] + suffix
}

// highlight wraps occurrences of q in <mark> tags.
func highlight(text, q string) string {
	if q == "" {
		return template.HTMLEscapeString(text)
	}
	var b strings.Builder
	ql := strings.ToLower(q)
	tl := strings.ToLower(text)
	i := 0
	for {
		idx := strings.Index(tl[i:], ql)
		if idx == -1 {
			b.WriteString(template.HTMLEscapeString(text[i:]))
			break
		}
		b.WriteString(template.HTMLEscapeString(text[i : i+idx]))
		b.WriteString(`<mark>`)
		b.WriteString(template.HTMLEscapeString(text[i+idx : i+idx+len(q)]))
		b.WriteString(`</mark>`)
		i += idx + len(q)
	}
	return b.String()
}
