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

### 2026-04-29 — Modern Flexcompute theme + dark/light toggle + SVG icons

User feedback: "more modern theme with Flexcompute color and option to
toggle dark and light there. Dark theme should be default. Use nicer
icons. Imagine you are an expert visual graphics person."

**Brand**: Pulled Flexcompute's primary green from their CSS:
`--brand-primary: #00643c`. Used that directly for the light theme accent
and a brighter sibling `#00b870` for dark backgrounds (the literal color
is too dim against `#0f1113`).

**Theme system**: Built around CSS custom properties. Two themes selected
via `data-theme="dark"|"light"` on `<html>`. A small inline `<script>` in
`<head>` reads `localStorage["lyre-theme"]` and sets the attribute *before*
first paint to avoid the flash-of-wrong-theme. Toggle is a sun/moon button
in the top bar that writes the new value back to localStorage.

**Icons**: Replaced every emoji (🐦, ✋, 🤖, 📦, etc.) with inline Lucide-style
SVG icons via a Go `iconHTML(name)` template func. Crisp at any size, line
style, theme-aware via `currentColor`. Icons used:
- `bird` for the Lyrebird logo mark
- `feather` for Lyrebird-actor avatar (system events)
- `user` for You-actor avatar
- `sparkles` for Claude-actor avatar
- `undo`, `plus`, `package` for the action buttons
- `file`, `chat`, `arrow-right`, `arrow-left` for inline UI
- `sun`/`moon` for the theme toggle
- `search` inside the header search box

**Visual hierarchy**:
- Larger, tighter h2 (28px / -0.02em letter-spacing) for the folder name
- Slimmer top bar with sticky positioning
- File pills now circular with the file icon inline
- Avatars are 32px circles with translucent tinted backgrounds matching
  each actor's color (warm amber for You, cool blue for Claude, brand
  green for Lyrebird)
- "show what changed" links got an arrow-right icon for affordance
- Subtle dividers (`--border-soft`) between story items inside a day,
  and stronger dividers (`--border`) between days

**Diff palette**:
- Dark: `#0d3318` add bg / `#4ec979` add fg, `#3d1418` del bg / `#ef7178`
  del fg, `#0a2e44` hunk band — all readable but muted enough to not yell
- Light: kept the GitHub colors (`#e6ffec`/`#1a7f37`, `#ffebe9`/`#cf222e`)
- Both themes pull from the same set of CSS variables; only the values
  differ

**Result**: same data, totally different feel. Dark theme is the default.

### 2026-04-28 — UX pass 2: cut visual busyness, fix theme-broken contrast

User feedback after the first UX pass:
- "Some color themes are still off"
- "This content is great but maybe still visually too busy"
- "I can barely see the 'Adding test_fib.py' color" (assistant text on the show page)

**Root cause for the contrast bug**: my old `show.html` used inline styles
like `style="background:#f0fbf2;border-left-color:#3a8b54"` — hardcoded
light-mode colors. The user had switched to dark mode (which they'd added a
toggle for, plus a full theme-token system in `_layout.html`), so the
prompt cards rendered as light-green tint with the inherited light text
color → barely visible. Same hardcoded-color trap on the file-history
`CURRENT` badge.

**Diagnosis for "too busy"**: each story item had FIVE visual elements
fighting for attention — avatar, actor name + sentence, prompt-box card,
file-chip row, and an explicit "show what changed →" link. That's a wall
of UI per row when the data is "X edited Y at Z."

**Fixes**:

1. **Theme-aware everywhere**. Removed every inline style that hardcoded a
   color. New `convo-card`, `convo-user`, `convo-assistant`, `convo-label`,
   `convo-text` classes on the show page that route through `--prompt-bg`,
   `--asst-bg`, `--text`, `--text-2`, `--actor-you`, `--actor-ai`. Same for
   file-history — `CURRENT` badge now uses `var(--actor-you)` so it pops
   in either theme.

2. **Inline filenames in the sentence** — new `renderHeadlineHTML`
   function returns `template.HTML` with each filename wrapped as
   `<a class="inline-file">`. Removed the redundant `.story-files` chip
   row entirely. The headline now reads "edited `fib.py` and
   `transcripts/sess1.jsonl`" with each filename as a clickable mono pill
   inline in the sentence.

3. **Quieter AI prompt** — replaced the boxed `prompt-line` card with a
   subtle `.story-quote`: italic, no background, just a thin
   `border-left` in the AI-blue color. One line of text, not a card.

4. **Demoted "show what changed"** — removed it from the visible flow
   entirely on the timeline. Each story item gets a small `.story-detail`
   arrow that's `opacity: 0` by default and reveals on row hover. The full
   row also has a hover background tint, so the click affordance is clear
   without permanent visual noise.

5. **Right-aligned time** — moved the `· 7 hours ago` to flow naturally at
   the end of the sentence rather than in a separate flex slot. Safer at
   narrow viewports where flex was breaking the headline into one-word-per-line
   stacks.

6. **Quieter system events** — `[lyre]` / `[safety]` events now use
   `.story-quiet` styling: smaller avatar (22px vs 28px), tighter padding
   (4px vs 10px), 0.55 opacity. They become a faint footnote in the story
   instead of competing for attention.

7. **File history tidied** — wrote a proper `.file-hero`, `.back-link`,
   `.file-mono` instead of inline-style soup. The two per-version actions
   ("show what changed", "bring back this version") render as quiet
   text-buttons with icons, no visible button chrome.

**Bug along the way**: putting `<a>` elements inside an `<a class="story-link">`
wrapper caused the browser to auto-close the outer link prematurely (HTML
spec — anchors can't nest). Reverted to a `<div>` row with inline links
inside; the row gets a hover-tint background plus a hover-revealed detail
arrow on the right, which gives the same "whole row is clickable-ish" feel
without the spec violation.

**Result**: a story item that used to render as

```
✋ You · Edited hello.py and notes.md · 6 hours ago
   hello.py  notes.md
   show what changed →
```

now renders as

```
✋ You  edited `hello.py` and `notes.md`  · 6 hours ago
```

Same information, half the visual weight, and the filenames are
themselves clickable. The "show what changed" arrow appears on hover
instead of always.

### 2026-04-28 — Visual metaphor: book chapters + spine + time travel

User: "the arrow to enter each edit is a bit conspicuous… some buttons to
restore checkpoint got deleted I think… would be cool to add some
delightful card mimicking a book being flipped or a scroll bar in timer
for each action."

Wrote `BRAINSTORM_VISUAL.md` with six metaphor options sketched. User picked
"go with your recommendation" — the hybrid (B + A + C):
- Vertical timeline with spine as the foundation (modern, scales)
- Book chapters at the day level (delightful, easy reading)
- Time-travel scrubber as a separate view (for moment-grabbing tasks)

**Implementation**:

1. **Chapter cards.** Each day group now renders as a `.chapter` card —
   slightly warmer surface tone (`--chapter-bg` token: `#181a1d` dark /
   `#fdfcf7` cream light), soft drop shadow, serif chapter heading
   ("Chapter N · Yesterday") in Georgia. Chapter numbers count from
   oldest (= Chapter 1) up to newest, so the "story so far" reads
   naturally as a numbered sequence.

2. **Vertical spine.** Inside each chapter, a 2px vertical line runs down
   the left edge. Each story item gets a `.node` — a small ring colored
   to the actor (amber=You, blue=Claude, green=Lyrebird) — sitting on the
   spine. On hover the node scales 1.25× and fills with the actor color,
   creating a satisfying "moment lights up" feedback.

3. **Always-visible quiet actions** replace the hover-only arrow.
   Each story row now has two text-buttons under the headline:
   `↺ show what changed` and `↶ rewind to here`. Both are quiet by
   default (text-dim color, no chrome) and pop on hover. The "rewind"
   one tints amber on hover to signal a state-changing action.

4. **`/rewind` endpoint.** New POST handler that auto-snapshots the
   current state, then reverts the work tree to a given hash, then
   commits the revert as `[revert] folder rewound to <short>`. Reversible
   like all destructive ops in Lyre.

5. **`Bring folder back to this state` on the snapshot detail page.**
   Primary action top-right of the show hero — closes the loop where
   you've drilled in to "what changed" and then want to actually rewind.

6. **Time-travel page (`/travel`).** Server emits the full event list
   oldest→newest as a JS array. A `<input type=range>` slider scrubs
   through it; on every `input` event the page fetches `/travel/state?hash=…`
   which returns the file list at that snapshot (via `git ls-tree -r
   --name-only <ref>`). The hero updates with "Actor · Headline" and
   "n hours ago · Apr 28 · 10:17 pm". A primary "Bring the folder back
   to this state" button on the same page makes the travel→restore loop
   one click.

   Slider styling uses a CSS custom property `--travel-pct` that the JS
   updates to render a green progress fill from start → thumb position.
   Shows "n hours ago" / "now" axis labels.

7. **Honest filename pills inline in the headline** (from pass 2) work
   even better in the new layout — they read as natural typography
   embedded in the chapter prose.

**The rewind UX path is now**:

| Where you are | How to rewind |
|---|---|
| Timeline (any event) | "↶ rewind to here" button on the event |
| Show page (snapshot detail) | "Bring folder back to this state" header button |
| File history | "Bring back this version" per-version button |
| Anywhere | "Travel" → drag slider → "Bring the folder back to this state" |
| Hero | "Undo" rolls back the most recent non-system change |

So restore is now reachable from every meaningful surface, and the most
expressive of them (Travel) lets you preview a moment before committing.

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
