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
		"truncate": func(s string, n int) string {
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
	return mux
}

type eventVM struct {
	Hash       string
	ShortHash  string
	Date       string
	Subject    string
	Files      []string
	IsAI       bool
	Agent      string
	SessionID  string
	UserPrompt string
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
			ev.Date = t.Format("2006-01-02 15:04")
		} else {
			ev.Date = e.Date
		}
		files, err := s.store.FilesChanged(e.Hash)
		if err == nil && len(files) > 0 {
			limit := 6
			if len(files) > limit {
				ev.Files = append(files[:limit], fmt.Sprintf("…+%d more", len(files)-limit))
			} else {
				ev.Files = files
			}
		}
		if sess, turn, _ := s.sess.FindByCommit(e.Hash); sess != nil && turn != nil {
			ev.IsAI = true
			ev.Agent = sess.Agent
			ev.SessionID = sess.SessionID
			p := turn.UserPrompt
			if len(p) > 240 {
				p = p[:240] + "…"
			}
			ev.UserPrompt = p
		}
		out = append(out, ev)
	}
	return out, nil
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
	flash := r.URL.Query().Get("flash")
	s.render(w, "timeline.html", map[string]any{
		"Title":  "Timeline",
		"Repo":   s.repo,
		"Events": events,
		"Flash":  flash,
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
	diff, _ := s.store.Show(match.Hash)
	sess, turn, _ := s.sess.FindByCommit(match.Hash)
	date := match.Date
	if t, err := time.Parse(time.RFC3339, match.Date); err == nil {
		date = t.Format("2006-01-02 15:04:05")
	}
	s.render(w, "show.html", map[string]any{
		"Title":   "Snapshot " + match.ShortHash,
		"Hash":    match.Hash,
		"Date":    date,
		"Subject": match.Subject,
		"Diff":    diff,
		"Session": sess,
		"Turn":    turn,
	})
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
		UserPrompt string
	}
	var versions []version
	for _, e := range entries {
		v := version{Hash: e.Hash, ShortHash: e.ShortHash, Subject: e.Subject}
		if t, err := time.Parse(time.RFC3339, e.Date); err == nil {
			v.Date = t.Format("2006-01-02 15:04")
		} else {
			v.Date = e.Date
		}
		if _, turn, _ := s.sess.FindByCommit(e.Hash); turn != nil {
			p := turn.UserPrompt
			if len(p) > 200 {
				p = p[:200] + "…"
			}
			v.UserPrompt = p
		}
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
