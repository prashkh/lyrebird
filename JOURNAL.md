# Lyrebird Build Journal

A running log of decisions, experiments, and dead-ends during the build.
Newest entries on top.

---

## 2026-04-28 — Day 0: Project setup

**Goal**: Build the Lyrebird MVP end-to-end so the user can try it in their next project.

### Decisions made

- **Folder layout**: Renamed `silent-vcs` → `lyrebird`. Repo root will live at `~/Documents/LocalRepos/lyrebird/`. Design docs (PROBLEM, BRAINSTORM, DESIGN) move into the repo as the historical record.
- **Repo**: Private, `prashkh/lyrebird` on GitHub.
- **Language**: Go, as committed in DESIGN.md. Installing Go via Homebrew since it wasn't on the machine.
- **License**: MIT for now (we can re-license later if we go AGPL/commercial).
- **Telemetry**: Zero, per default.
- **Branch model**: `main` only for now. Phase commits will be small and frequent so the history itself becomes the build narrative.

### Build plan (this session)

Targeting MVP phases 1-6 from DESIGN.md:

1. CLI skeleton + git wrapper
2. FS watcher daemon
3. Claude Code adapter
4. Handoff packager (priority — user's smell test)
5. Minimal web UI
6. End-to-end test in a sandbox folder

Phases 4 (Codex CLI), 8 (Cursor), 9 (VS Code) deferred to a follow-up build.

### Open questions (resolved as I went, listed here for the record)

- Go vs Python for v0? → Go, matches design, installable in 30s.
- Watcher: foreground or daemonized? → v1 ships as a foreground `lyre watch` you can also background via launchd; full launchd plist is a stretch goal.
- UI: SPA or HTMX? → Server-rendered HTML with embedded CSS. No HTMX needed for v1 — full page navigations are fast enough on localhost. Can layer HTMX on later for partial reloads.

### Phase-by-phase notes

**Phase 1 — CLI skeleton** ✅
- Hand-rolled CLI dispatch, no Cobra. Less polish, zero deps, ~80 LOC.
- Hidden git repo at `.lyrebird/git/` with `GIT_DIR` + `GIT_WORK_TREE` env. Confirmed: doesn't collide with the user's own `.git/`.
- `git init -b lyre-snapshots` so our branch is named for what it is.
- Restore + revert auto-snapshot first → reverting is itself reversible. This is the property that makes silent automation safe.
- Decision: JSON config instead of TOML. Standard library, fewer deps. Easy to switch later if the schema gets complex.

**Phase 2 — Watcher** ✅
- `fsnotify` (cross-platform). Walked tree at start, added every dir; on Create-of-dir events, watch the new dir.
- 2s debounce default (configurable via `--debounce`).
- Hard-coded ignore list at the watcher level for `.lyrebird`, `.git`, `.venv`, `node_modules`, `__pycache__`, `.ipynb_checkpoints`, plus editor swap files.
- The git layer's `.lyreignore` (via `core.excludesFile`) is the second line of defense — even if the watcher fires, git might decline to commit.
- Decision: skipped daemon-ization. v1 is `lyre watch &` in a terminal. launchd plist is straightforward to add later.

**Phase 3 — Claude Code hook** ✅
- `lyre hook` reads JSON from stdin (Claude Code's `PostToolUse` payload format), walks up from `cwd` to find the lyrebird repo, takes a snapshot, persists session metadata.
- Tails `transcript_path` JSONL to grab most recent user prompt + assistant text. Best effort — handles both string content and structured content blocks.
- Decision: only act on `Edit`/`Write`/`NotebookEdit`/`MultiEdit`. Read tools (Read, Grep, Glob, Bash without write) are noise.
- Decision: hook fails silently to stderr. Never block the agent.
- `lyre install-hook` patches `~/.claude/settings.json` idempotently.

**Phase 6 — Handoff packager** ✅
- `lyre handoff` produces `HANDOFF.md`, `CONTEXT.md`, `files/` (via `git archive` + tar pipe), `sessions/` (verbatim copy), `timeline.json`.
- `HANDOFF.md` is **deterministic** — no LLM call, just renders from git log + session JSONs. The receiving AI does its own narrative if needed.
- Test verified: handoff for the hook-test sandbox correctly preserves the user prompt alongside the file change it caused.

**Phase 5 — Web UI** ✅
- Embedded HTML templates via `go:embed`. Single static binary, no runtime asset paths.
- `html/template` with shared `_layout.html` + per-page templates.
- All inline styles in `_layout.html` — single source of truth, no external CSS file, offline-friendly.
- Pages: timeline (home), sessions list, session detail, snapshot detail (diff + chat), file history, search, handoff result.
- Decision: server-rendered, full page navigations. HTMX deferred — the localhost roundtrip is <10ms anyway.
- Verified end-to-end via the preview tool: timeline → snapshot detail → session view → handoff. All routes return 200, pages render correctly with chat-thread attribution.

### End-to-end verification

Ran a scripted scenario in `/tmp/lyre-e2e` that exercises every piece:

1. `lyre init` → initial snapshot
2. Started `lyre watch` in the background
3. Manually created `README.md` → watcher caught it (snapshot `9cedc05`, `[manual]`)
4. Simulated Claude Code session `sess_aaa_111`, turn 1: "Create a Python script that prints fibonacci numbers" → `lyre hook` snapshot `0db834a`, `[ai]`
5. Same session, turn 2: "Add a main block that prints first 10" → snapshot `798dcc5`
6. Different session `sess_bbb_222`, turn 1: "Add a test for the fib function using pytest" → snapshot `8e0226c`
7. Manually wrote `scratch.md` → watcher caught it (snapshot `d08bd8c`, `[manual]`)
8. `lyre restore fib.py 0db834a` → rolled fib.py back to first AI version, auto-snapshotted current state, then committed the restore as snapshot `df66f54`
9. `lyre handoff` → produced a complete package with HANDOFF.md, CONTEXT.md, files/, sessions/, timeline.json
10. `lyre ui` → timeline shows all 7 events with manual/AI badges, chat snippets, links to diffs and full sessions
11. Search for "fibonacci" → matches the chat thread, highlights the word

Every piece works as designed. The handoff `timeline.json` correctly attributes
each AI snapshot to a `(session_id, turn_id, user_prompt)` tuple while leaving
manual snapshots untagged.

### Things deferred (write down so we don't forget)

- Notebook stripping (jupytext sidecar). Currently `.ipynb` diffs are noisy.
- Codex CLI adapter (tail `~/.codex/sessions/`).
- Cursor adapter (tail SQLite `state.vscdb`).
- VS Code extension (Continue, Cline, Copilot Chat capture).
- launchd plist for daemon lifecycle.
- Cloud sync (`lyre remote add`, `lyre push`).
- Pre-sync secret scrubber.
- `curl ... | sh` installer script.
- Cross-platform binary builds + Homebrew formula.
