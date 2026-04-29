# Brainstorm — Names & Solution Shapes

## Names (animal-based)

Vibe target: silent, watchful, archives/collects, never forgets. Bonus if the
CLI command is one short syllable.

### Top picks

| Name | Why it fits | CLI |
|------|---|---|
| **Lyrebird** | Australian bird famous for *perfectly recording and replaying any sound it hears* — chainsaws, camera shutters, other birds. Literal metaphor for "records everything that happens in your folder." | `lyre` |
| **Bowerbird** | Builds a "bower" — a curated collection of objects, meticulously arranged. Metaphor for archiving the artifacts of a session. | `bower` |
| **Magpie** | Smart, observant, collects shiny things. Fun, recognizable. | `mag` or `magpie` |
| **OwlWatch / Hoot** | Owls fly silently, see in the dark, watch from above. Closest to your "owl watch" example. | `hoot` |
| **Mole** | Burrows quietly underground (= lives inside your folder), barely visible. | `mole` |

### Honorable mentions

- **Packrat** — hoards everything (a little pejorative).
- **Squirrel / Acorn** — caches for later. Friendly.
- **Caddisfly** — insect that builds a protective case from collected debris (cool but obscure).
- **Salmon** — returns home (= revert). Too obscure.
- **Mockingbird / Parrot** — records and replays. Lyrebird is the better version of this idea.

### My recommendation

**Lyrebird** (`lyre` as the CLI). Three reasons: (1) the record/replay
metaphor is exact, not just thematic; (2) it's distinctive and not in heavy
use as a software name; (3) "lyre" as a 4-letter CLI is short to type and
unambiguous on the command line. Will use **Lyrebird** as the working name
for the rest of this doc — easy to global-replace later.

---

## Solution shapes

> **Decisions locked in (2026-04-28):**
> - **Storage = git, under the hood.** Don't reinvent. Use a hidden git repo (separate `--git-dir`) inside `.lyrebird/`. Auto-commit on every detected change. All history-exploration commands wrap git equivalents.
> - **Capture = hybrid.** A filesystem watcher catches *every* change (manual or AI). Claude Code hooks fire alongside the watcher to enrich AI-driven changes with chat-thread metadata.
> - **Binaries are stored, not skipped.** Restoration is the priority over disk-efficiency. Optimize later.

The requirements bucket into three layers:

1. **Capture** — how do file changes and chat threads get recorded?
2. **Storage** — where does the history live, and in what format?
3. **Sync / share** — how does it leave the laptop (later)?

### Capture: three options

#### A. Filesystem watcher (daemon)
- Background process uses macOS FSEvents / Linux inotify / Windows ReadDirectoryChangesW.
- Watches every configured root. On any write, snapshots the file.
- ✅ Catches *all* edits — AI, human, IDE auto-format, anything.
- ❌ Daemon process to install and keep running.
- ❌ No native link to "which chat turn caused this write." We have to *infer* it by timestamp-matching against the agent's transcripts.

#### B. Agent hooks (no daemon)
- Hook into the AI agent itself: Claude Code's `PostToolUse` hook fires on every `Edit`/`Write`/`NotebookEdit`. The hook is just a small `lyre snapshot` shell command we ship.
- The hook receives the full tool-call payload including the session ID, so we get a *native* link from "this file change" to "this chat turn."
- ✅ No daemon. No process management.
- ✅ Perfect attribution — we know exactly which prompt, which tool call, which file write.
- ❌ Only catches AI activity, not manual edits.
- ❌ Has to be wired up per agent (Claude Code today; Cursor / Aider / Copilot would each need their own integration).

#### C. Hybrid: hooks for AI activity + watcher for everything ← **chosen**
- Watcher (FSEvents on macOS, `fsnotify`/`notify` cross-platform) sees **every** write — AI, human, IDE auto-format, generated artifacts. Each write becomes a git commit.
- Claude Code `PostToolUse` / `Stop` hooks fire in parallel and dump session ID, turn ID, prompt, tool args into `.lyrebird/sessions/<session-id>/`. The watcher's commit picks these up and embeds them in the commit message + a JSON sidecar.
- Manual edits get committed too — just with `[manual]` in the message and no chat metadata.
- Result: complete history, with rich AI attribution where available and basic timestamp attribution everywhere else.

### Storage: git under the hood

Decision: don't reinvent. Use git as the actual object store. The trick is to keep our git repo **separate** from any user-owned git repo in the same folder.

```
~/myproject/
├── .git/               ← user's own repo (if any) — UNTOUCHED
├── .lyrebird/
│   ├── git/            ← our hidden repo (GIT_DIR points here)
│   ├── sessions/       ← per-session JSON: prompts, tool calls, transcripts
│   ├── config.toml     ← per-folder lyre config
│   └── lyrebird.db     ← SQLite index for fast queries (rebuildable from git)
├── .lyreignore         ← like .gitignore, lyre-specific
└── ...your files...
```

Every `lyre` command sets `GIT_DIR=.lyrebird/git GIT_WORK_TREE=.` and shells out to git. The user's own `.git/` (if any) is invisible to lyre and vice versa.

| Why git | Concern | Mitigation |
|---|---|---|
| Battle-tested object store with content-addressed dedup | Big binaries | v1: just store them. Git deduplicates identical blocks. A 7 MB hdf5 stored 10 times is **not** 70 MB if blocks repeat. v2: tier large blobs to cloud / use git-LFS-style external store. |
| `git log`, `git show`, `git diff`, `git checkout` give us the entire history-exploration UX for free | Auto-commit churn | Use a single dedicated branch (`refs/heads/lyre-snapshots`) so noise is invisible from the user's main branch (we don't share refs anyway). |
| Push-to-remote is one command | `.ipynb` diffs are noisy | Strip outputs into a sidecar `.ipynb.lyre.py` (jupytext) on commit. Display the clean diff; restore the full notebook on `lyre restore`. |
| Cross-platform, ubiquitous on dev machines | What if user later runs `git status` in the folder | Our git lives in `.lyrebird/git/` and is invisible to their git. Zero collision. |

### Notebook handling

`.ipynb` is JSON with embedded outputs that change every run, which makes
diffs useless. Strategy: **store both**.
- Raw `.ipynb` bytes go into the git commit (fidelity, restorable).
- A normalized `jupytext` version is committed alongside (used for diffs and search).
- `lyre diff` shows the jupytext diff by default; `--raw` shows the JSON.
- `lyre restore` always brings back the full `.ipynb` with outputs intact.

### Revert & explore — the interface

This is critical UX. Three layers, growing in fidelity:

#### Layer 1 — CLI (v1, ships first)

Mostly thin wrappers around git. The whole point of using git underneath is to inherit this UX cheaply.

```
lyre log                    # timeline of sessions+changes (like git log, but grouped by session)
lyre log <file>             # history of just one file
lyre show <id>              # what changed in this snapshot, plus the chat thread that caused it
lyre diff <id-a> <id-b>     # diff between two points (file or whole folder)
lyre sessions               # list AI sessions, with prompt summaries
lyre session <id>           # full chat transcript + every file that session touched
lyre search "route_manhattan"  # find every chat / file referencing a phrase

lyre restore <file> <id>    # bring one file back from a prior snapshot (non-destructive — current state is auto-snapshotted first)
lyre revert <id>            # roll the WHOLE folder back to a prior state (auto-snapshots current state first; reversible)
lyre fork <id>              # create a branch starting from a prior point — explore an alternate history without losing current
```

**Critical safety property**: every `restore` and `revert` *first* takes a snapshot of the current state, so nothing is ever lost. "Reverting" is itself reversible — you can revert the revert. This is the trust property that makes `lyre` safe to leave running.

#### Layer 2 — Local web UI (v1.5)

`lyre ui` opens `http://localhost:6789` with:
- **Timeline view**: horizontal axis = time, rows = files, dots = changes. Color by session. Hover a dot → tooltip with chat snippet.
- **Session view**: pick a session → see the full chat thread on the left, the files it touched on the right, with diffs.
- **File view**: pick a file → see all its versions stacked, with side-by-side diff and a "restore this version" button.
- **Notebook rendering**: `.ipynb` versions render in-browser so you can see actual cell outputs at each point in history.
- **Search**: full-text across chats and file contents.

This is the killer feature for "where did this file come from" — the timeline + chat thread together.

#### Layer 3 — IDE / agent integration (later)

- A Claude Code slash command: `/lyre log` shows recent snapshots inside the chat.
- VS Code extension shows lyre history in a sidebar, like git blame but session-aware.

### What happens to the folder when you revert?

Same model as git, with extra safety:

- **`lyre restore <file> <id>`** — single file is overwritten with the historical version. Other files untouched. Your current state is auto-snapshotted first.
- **`lyre revert <id>`** — entire tracked folder reverts to the state at snapshot `<id>`. *Current state is auto-snapshotted first* (committed as a snapshot tagged `before-revert-<timestamp>`), so you can always come back.
- **`lyre fork <id>`** — creates a new branch starting from `<id>`. Doesn't touch your working tree until you switch to it. Good for "explore an alternate path without losing what I have."

So yes: it works like git. But because we always auto-snapshot before any destructive op, you never lose work. That's the property that lets you actually trust it.

### Sync / share (future, but design for it now)

Local-first, with three optional escalation tiers:

1. **Local only** (default). `.lyrebird/` lives in the folder.
2. **Personal cloud backup**. Push `.lyrebird/` to S3 / R2 / Backblaze. One-time `lyre cloud login`. Snapshots upload in the background. Restore on a new machine = `lyre clone <folder-id>`.
3. **Team sharing**. Two sub-modes:
   - **Snapshot share**: `lyre export <session-id>` produces a single self-contained file (folder state + chat) that you send to a teammate. They run `lyre import` and get a read-only view.
   - **Live channel**: a folder is shared continuously to a small group. Teammates see each other's sessions. Conflict resolution becomes interesting (probably read-only-by-default with explicit fork).

Importantly: cloud and team modes are **opt-in upgrades**, not the v1
storage backend. v1 ships without them.

### Privacy

Chat transcripts may contain API keys, file paths, customer names. Defaults:
- Everything stays under `.lyrebird/` (local).
- Pre-sync scrubber checks for high-entropy strings, common secret patterns; flags them before any cloud upload.
- `.lyreignore` (mirrors `.gitignore` semantics) for files you *never* want captured.

---

## Install & activation UX

The "one curl" goal:

```bash
curl -fsSL https://lyrebird.dev/install.sh | sh
```

This installs a single static binary `lyre` to `~/.local/bin/lyre` and
optionally registers a Claude Code hook in `~/.claude/settings.json`.

After install, three modes:
1. **Explicit init**: `cd somefolder && lyre init` — start tracking.
2. **Auto-attach** (opt-in): `lyre auto on` registers a Claude Code hook that
   detects when an agent first writes a file in a non-tracked folder and
   prompts (one-time) "track this folder?"
3. **Always-on** (power user): `lyre auto on --aggressive` — every folder
   under `~/Documents/LocalRepos` is tracked silently.

---

## Concrete v1 design (the MVP spec)

A focused first version we can build and you can try in your next project.

### Components

1. **`lyre` binary** (single static, Go or Rust). Subcommands: `init`, `watch`, `log`, `show`, `diff`, `sessions`, `session`, `search`, `restore`, `revert`, `fork`, `ui`.
2. **Watcher daemon** — started by `lyre init` via a launchd plist on macOS (later: systemd unit on Linux). Reads `~/.lyre/registry.toml` for the list of tracked folders. Uses `fsnotify`/`notify` to watch each.
3. **Claude Code hook** — `~/.claude/settings.json` gets a `PostToolUse` hook calling `lyre hook --tool $TOOL --session $SESSION_ID --turn $TURN_ID --payload @-` after every `Edit`/`Write`/`NotebookEdit`. Hook writes session metadata into `.lyrebird/sessions/`.
4. **Per-folder `.lyrebird/`** — hidden git repo, sessions dir, SQLite index, config, `.lyreignore`.

### Commit model

- Every detected change (debounced ~2s) → one `lyre snapshot` action.
- Snapshot creates a git commit on `refs/heads/lyre-snapshots`.
- Commit message is structured:
  ```
  [ai|manual] <session-id-short> <file-list>
  
  prompt: <first 200 chars of the user prompt that triggered this>
  tool: Edit
  turn: <turn-id>
  ```
- Sidecar JSON in `.lyrebird/sessions/<session-id>/<turn-id>.json` holds the full chat-turn payload.

### Default ignore rules

`.lyreignore` ships with sensible defaults:
- `.venv/`, `node_modules/`, `__pycache__/`, `.ipynb_checkpoints/`
- `.DS_Store`, `*.pyc`
- (We do **not** skip `.hdf5`, `.gds`, `.png` by default — your requirement.)

### Install flow

```bash
curl -fsSL https://lyrebird.dev/install.sh | sh
# Installs `lyre` to ~/.local/bin
# Drops a launchd plist at ~/Library/LaunchAgents/dev.lyrebird.watcher.plist
# Patches ~/.claude/settings.json to register the PostToolUse hook (with confirmation)
# Prints next steps
```

```bash
cd ~/Documents/LocalRepos/MyNextProject
lyre init
# Creates .lyrebird/, runs `git init` inside it, registers folder with daemon
# Daemon picks it up immediately, starts watching
```

That's it. From here on, everything is silent. `lyre log` whenever you want to look back.

### Build order

1. **Day 1**: `lyre init`, `lyre snapshot <file>` (manual), `lyre log`, `lyre show`, `lyre restore`. No daemon, no hook. Pure CLI sanity check on top of git.
2. **Day 2**: Watcher daemon — auto-snapshot on file change. Now it works for manual edits.
3. **Day 3**: Claude Code hook — chat metadata starts flowing. `lyre session <id>` shows transcripts.
4. **Day 4**: `lyre revert`, `lyre fork`, search.
5. **Day 5+**: Web UI. Cloud sync. Cursor support.

That's roughly the scope of an MVP you could try in your next project.

---

## Resolved (2026-04-28)

All major questions resolved. Forward-looking spec is in [DESIGN.md](./DESIGN.md).

| Question | Resolution |
|---|---|
| Granularity | **Per file-write** (max granularity, cheap with git, easy to roll up in UI) |
| MVP smell test | **`lyre ui` to see everything done + `lyre handoff` to package folder for another AI.** Web UI and handoff are now v1 features, not v1.5. |
| Cloud sync | **Bring-your-own-bucket** via `git push` to a remote helper (S3/R2/etc.). No hosted service. |
| Multi-agent support | Claude Code (hooks), Codex CLI (log tail), Cursor (SQLite tail), VS Code (extension). All in scope; build order specified in DESIGN.md. |
| Naming | **Lyrebird** / `lyre`. `lyre` is taken on npm/PyPI/crates.io but we ship as a static binary so it doesn't block us. |
