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

### 2026-04-28 — install.sh follow-up

User hit `zsh: command not found: lyre` after the build. The binary was at
`bin/lyre` inside the repo, but never copied to anywhere on `$PATH`.

**Fix**:
- Copied the existing binary to `/opt/homebrew/bin/lyre` for the immediate fix.
- Wrote `install.sh` for the proper one-command UX going forward. It:
  1. Picks the first writable directory on `$PATH` (`~/.local/bin` →
     `/opt/homebrew/bin` → `/usr/local/bin`), honors `$LYRE_INSTALL_DIR`.
  2. Builds with `go build -ldflags '-s -w'` (smaller binary).
  3. Verifies the install dir is on `$PATH`; warns with the exact zshrc line
     if not.
  4. Offers to register the Claude Code PostToolUse hook (auto-yes if stdin
     isn't a TTY, e.g. piped from curl).
- README updated with the actual one-command install:
  `gh repo clone prashkh/lyrebird /tmp/lyrebird && /tmp/lyrebird/install.sh`
- Note: while the repo is private, plain `curl | sh` won't work without
  authentication. Once we cut a public release (or set up GitHub releases
  with prebuilt binaries), the install line becomes
  `curl -fsSL https://lyrebird.dev/install.sh | sh`.

### 2026-04-28 — UI overhaul: from git frontend to a story

**Trigger**: User opened the UI in their `Lyrebird_test` folder and said
"this UI is very opaque to an average user — lots of details but no summary
of what happened." Screenshot showed:
- `7d8621e 2026-04-28 22:28 [manual] view diff →`
- Editor temp files (`notes.md.tmp.1860.1777429611772`) leaking through into
  commit subjects.
- A flat list with no narrative.

**Diagnosis**: I'd built a git frontend dressed up. Words like *snapshot*,
*hash*, *diff*, *manual* mean nothing to a non-developer. The page was an
audit log, not a story.

**Redesign**:

1. **Vocabulary swap**: Removed all developer jargon from user-facing strings:
   - "snapshot" → "change" / "save"
   - "hash" → "id" (and de-emphasized to `dim small`)
   - "diff" → "what changed in the files"
   - "revert" → "undo"
   - "restore" → "bring back this version"
   - "session" → "conversation"
   - "manual" → "You" (with ✋ avatar)
   - "claude-code" → "Claude" (with 🤖 avatar)
   - System events ([lyre]/[safety]/[restore]/[revert]) → "Lyrebird" actor (🐦)

2. **Story view**: Replaced the flat list with a day-bucketed narrative:
   - Day headers: "Today", "Yesterday", "Tuesday", "Apr 12"
   - Each event reads as a sentence: "**You** edited hello.py and notes.md · 6 hours ago"
   - Actor avatars + colors (warm for you, blue for Claude, gray for Lyrebird)
   - Quiet system events ([lyre], [safety]) hide the file chips and "show what changed" link

3. **Big actions front-and-center**:
   - **↶ Undo** — rolls back the most recent non-system change (with confirm); auto-snapshots first so it's reversible.
   - **＋ Save now** — manual save with optional note, in a `<details>` popover.
   - **📦 Hand off** — primary action, package for another AI.

4. **Friendly hero block** instead of toolbar:
   - Folder name as h2
   - One-line summary: "3 changes from Claude · 4 from you · across 2 conversations · last change 6 hours ago"
   - File chips below as a quick visual inventory

5. **Headline rendering**:
   - Raw subject `[manual] hello.py notes.md` → "Edited hello.py and notes.md"
   - Raw subject `[ai] claude-code sess_abc fib.py` → "Edited fib.py" (actor "Claude" added separately)
   - Raw subject `[lyre] initial snapshot at lyre init` → "Started tracking this folder"
   - Raw subject `[restore] notes.md from 02d5cf0` → "Brought notes.md back to an earlier version"
   - Raw subject `[safety] before undo` → "Saved a checkpoint before undoing"

6. **Filter Lyrebird-internal files** from the displayed file lists (e.g.
   `.lyreignore`) so they don't appear as if they're user files.

7. **Onboarding state**: Empty folder shows "Just started. Edit some files
   to begin your story." plus a "Today" section with the welcome event.

8. **Detail pages**: Snapshot detail and per-file pages got the same
   vocabulary treatment. File page: "📄 hello.py · 2 versions of this file"
   with a "CURRENT" badge on the latest, "Bring back this version" buttons
   on prior ones.

9. **Editor temp files**: Filtered `*.tmp.*`, `*~`, `.#*`, `.swp`, etc. from
   both the watcher (so they don't trigger snapshots) and the UI display
   (defense in depth — filter even if they slip into history).

10. **Bug along the way**: Go template pipelines pass the piped value as the
    LAST argument. `{{.X | truncate 100}}` calls `truncate(100, .X)`, not
    `truncate(.X, 100)`. Got "expected string; found 100" until I flipped
    the signature.

**Result**: Side-by-side comparison of the same data:

> Before:
> `7d8621e 2026-04-28 22:28 [manual] · view diff →`
> `[manual] notes.md notes.md.tmp.1860.1777429611772`

> After:
> ```
> Lyrebird_test
> 5 changes, all yours so far · last change 6 hours ago
>   [↶ Undo]  [＋ Save now]  [📦 Hand off]
> Files in this folder: hello.py · notes.md
>
> YESTERDAY
>   ✋ You · Edited hello.py and notes.md · 6 hours ago
>      hello.py  notes.md
>      show what changed →
>   🐦 Lyrebird · Started tracking this folder · 6 hours ago
> ```

### 2026-04-28 — GitHub-style colored diff view

User feedback on the "show what changed" page: "can we make it see the diff
with red and green color more like how it's done traditionally?"

The previous render dumped raw `git show` output as monospace text on a
dark background — technically correct, visually a wall.

**Changes**:

1. New `gitstore.ShowPatch()` and `ShowStat()` use `git show --format=` to
   strip the commit header (we already render actor + headline + date above,
   so the duplicate metadata was noise).
2. `parseDiff()` walks the unified-diff output and emits structured
   `DiffFile` groups. Each line is classified as `add`, `del`, `hunk`, or
   `context`. File-level metadata (`index`, `--- a/`, `+++ b/`, `old mode`,
   `new mode`, `similarity`) is hidden — we already show the path, and a
   small badge tells you `new file` / `deleted file` / `renamed` / `binary`.
3. New CSS in `_layout.html` styles a GitHub-flavored diff:
   - Each file is its own card (`#fff` background, light gray border, file
     name in the header).
   - Add lines: green tint (`#e6ffec` background, `#1a7f37` text, `+` marker).
   - Del lines: red tint (`#ffebe9` background, `#cf222e` text, `−` marker).
   - Hunk separator: light blue band (`#ddf4ff`).
   - Context lines: plain white.
   - Diffstat (`foo.py | 5 +++--`) sits above the file cards in a small
     monospace block.
4. Binary files are detected and replaced with "Binary file — not shown."
   instead of showing garbage.

Result: the same data now reads like a GitHub PR. Tested on:
- A pure-add commit (Claude's `if __name__ == "__main__"` block).
- A pure-delete commit (the `[restore] fib.py` undo).
- The combined diff includes both colors when both exist.

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
