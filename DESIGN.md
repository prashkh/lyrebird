# Lyrebird — v1 Design

> Status: design locked. Build can start.
> See [PROBLEM.md](./PROBLEM.md) for motivation, [BRAINSTORM.md](./BRAINSTORM.md) for rationale.

## Name & branding

- **Project name**: Lyrebird (the bird that records and replays sounds perfectly)
- **CLI binary**: `lyre`
- **Repo**: `github.com/<org>/lyrebird`
- **Install URL**: `lyrebird.dev` (placeholder; domain TBD)

**Caveat**: `lyre` is taken on npm (stale GraphQL client), PyPI (empty squat),
and crates.io (audio library). Since we ship as a curl-installed static binary,
this doesn't block us — but if we ever publish to a registry, we'd use
`@lyrebird/cli`, `lyrebird-cli`, or similar.

## Killer feature (the MVP smell test)

> "Open `lyre ui`, see everything that was done in this folder, and hand the folder + a summary to another AI."

Two things follow from this:

1. The **web UI is part of v1**, not a v1.5 stretch goal.
2. There's a first-class **handoff** workflow — not just "explore history," but "package a folder so another AI can pick up where the last one left off."

`lyre handoff` produces a directory (or zip) containing:

```
handoff-2026-04-28/
├── HANDOFF.md           # Deterministic summary: what was tried, files & roles, sessions, open threads
├── CONTEXT.md           # System-prompt-shaped intro for the receiving AI
├── files/               # Current state of the tracked folder
├── sessions/            # Per-session JSON: prompts, tool calls, transcripts
└── timeline.json        # Machine-readable event log
```

`HANDOFF.md` is generated deterministically from the git history + session
metadata — no LLM call required. The receiving AI reads `CONTEXT.md` and can
ask its own questions of `sessions/` and `timeline.json`.

## Architecture

### Three-layer model

```
┌──────────────────────────────────────────────────────┐
│  Capture                                             │
│  ┌────────────────────┐  ┌────────────────────────┐  │
│  │ FS watcher         │  │ Per-agent adapters     │  │
│  │ (catches every     │  │ (enrich AI events with │  │
│  │  file write)       │  │  chat metadata)        │  │
│  └─────────┬──────────┘  └──────────┬─────────────┘  │
│            └──────────┬──────────────┘               │
│                       ▼                              │
│              [snapshot event]                        │
└─────────────────────────────────────────────────────┬┘
                       │                              │
┌──────────────────────▼──────────────────────────────┐│
│  Storage                                             │
│  .lyrebird/                                          │
│   ├── git/        ← actual git repo (separate dir)  │
│   ├── sessions/   ← JSON sidecars per AI session    │
│   └── lyrebird.db ← SQLite index for fast queries   │
└──────────────────────┬──────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────┐
│  Surface                                            │
│  ┌────────┐  ┌──────────┐  ┌─────────────────────┐  │
│  │ CLI    │  │ Web UI   │  │ Handoff packager    │  │
│  │ `lyre` │  │ localhost│  │ `lyre handoff`      │  │
│  └────────┘  └──────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

### Capture layer

**Watcher** (always-on, agent-agnostic):
- macOS: FSEvents, Linux: inotify, Windows: ReadDirectoryChangesW
- Cross-platform via Go's `fsnotify` or Rust's `notify`
- Debounces writes (~2s) before snapshotting
- Catches every file change — manual edits, IDE auto-save, AI tool calls, generated artifacts

**Per-agent adapters** (enrich snapshots with chat-thread context):

| Agent | Mechanism | Notes |
|---|---|---|
| **Claude Code** | `PostToolUse` hook in `~/.claude/settings.json` | Native session ID, turn ID, tool args, prompt all available. Easiest. |
| **Codex CLI** (OpenAI) | Tail `~/.codex/sessions/*.jsonl` | Codex CLI logs every turn to disk; we poll/tail and emit events. |
| **Cursor** | Tail SQLite at `~/Library/Application Support/Cursor/User/workspaceStorage/<hash>/state.vscdb` | Cursor stores chat in this DB. Poll on a 1-2s timer; correlate with file writes by timestamp. |
| **VS Code** (Continue, Cline, Copilot Chat, etc.) | Lyrebird VS Code extension | Registers as an MCP server / chat participant; emits events to the lyre daemon over a local socket. |

The watcher always works. Adapters are best-effort — when an adapter is
present, the snapshot gets rich metadata; when not, it's stored as `[manual]`
with timestamp only.

**Build order for adapters**: Claude Code → Codex CLI → Cursor → VS Code.

### Storage layer

Each tracked folder owns:

```
myproject/
├── .lyrebird/
│   ├── git/              # GIT_DIR — hidden git repo
│   ├── sessions/         # <session-id>/<turn-id>.json — chat metadata
│   ├── config.toml       # per-folder lyre config
│   ├── lyrebird.db       # SQLite index (rebuildable from git)
│   └── handoffs/         # generated handoff packages
├── .lyreignore           # like .gitignore, lyre-specific
└── ...your files...
```

**Git config**: every `lyre` git command runs with `GIT_DIR=.lyrebird/git
GIT_WORK_TREE=.`. Snapshots commit to `refs/heads/lyre-snapshots`. The user's
own `.git/` (if any) is untouched.

**SQLite index** stores: sessions (id, agent, started_at, ended_at,
prompt_summary), turns (session_id, turn_id, prompt, tool_calls), files
(path, last_snapshot_id), full-text index on chats and file contents.
Rebuildable from git + sessions/ at any time, so it's a cache, not the truth.

**Commit granularity**: per file-write. Every `Edit`/`Write` from the AI
becomes one commit. Every debounced manual save becomes one commit. Max
granularity, cheap with git, easy to roll up in the UI.

**Commit message format**:
```
[ai|manual] <session-short> <turn-short> <files...>

prompt: <first 200 chars of user prompt that triggered this turn>
tool: Edit
agent: claude-code
```

**Sidecar JSON** at `.lyrebird/sessions/<session-id>/<turn-id>.json`:
```json
{
  "session_id": "...",
  "turn_id": "...",
  "agent": "claude-code",
  "timestamp": "2026-04-28T20:11:32Z",
  "user_prompt": "...full text...",
  "assistant_text": "...full text...",
  "tool_calls": [{"tool": "Edit", "file": "charge.py", "input": {...}}],
  "files_changed": ["charge.py"]
}
```

**Default `.lyreignore`**:
- `.venv/`, `node_modules/`, `__pycache__/`, `.ipynb_checkpoints/`
- `.DS_Store`, `*.pyc`, `*.pyo`
- Binaries (`.hdf5`, `.gds`, `.png`, etc.) are **NOT** skipped — they're stored. Restoration is the priority.

**Notebook handling**: store both raw `.ipynb` and a jupytext-stripped
`.ipynb.lyre.py` sidecar. Diffs use the sidecar; restoration uses the raw
bytes.

**Safety property**: every destructive op (`restore`, `revert`) auto-snapshots
the current state first. Reverting is itself reversible.

### Surface layer

#### CLI (v1)

```
lyre init                       # Set up tracking in current folder
lyre status                     # Show daemon status, last snapshot, untracked files

# Exploration
lyre log [<file>]               # Timeline grouped by session
lyre show <id>                  # Diff + chat thread for a snapshot
lyre diff <id-a> <id-b>         # Diff between two points
lyre sessions                   # List AI sessions with prompt summaries
lyre session <id>               # Full chat transcript + files touched
lyre search <query>             # Full-text across chats and file contents

# Restore
lyre restore <file> <id>        # Bring one file back; current state auto-snapshotted first
lyre revert <id>                # Roll whole folder back; current state auto-snapshotted first
lyre fork <id>                  # Create branch from a prior point

# Surfaces
lyre ui                         # Open the local web UI
lyre handoff [--output <path>]  # Package folder + summary for another AI

# Sync (optional)
lyre remote add <name> <url>    # e.g. `lyre remote add backup s3://my-bucket/lyrebird/`
lyre push <remote>              # `git push` under the hood
lyre pull <remote>

# Daemon mgmt
lyre daemon start | stop | status | logs
```

#### Web UI (v1) — `lyre ui`

Opens `http://localhost:6789` in the user's browser. Local-only. No auth.

**Views**:
- **Timeline** (homepage): horizontal axis = time; rows = files; dots = changes; color by session. Hover for chat snippet preview.
- **Session view**: chat thread on left (collapsible turns), files-touched panel on right with diffs.
- **File view**: every version of a file stacked, side-by-side diff between adjacent versions, "Restore this version" button. Notebooks render with cell outputs at each point.
- **Search**: full-text across chats and file contents.
- **Handoff button**: prominent. One click → generates a handoff package, shows a summary preview, copies a path to the package.

#### Handoff packager (v1)

`lyre handoff` produces a self-contained directory (or `--zip` for a single file). Contents already specified above. The `HANDOFF.md` is generated deterministically — no LLM call — and looks like:

```markdown
# Handoff: MyProject (2026-04-28)

## Summary
- Tracked since: 2026-04-15
- Sessions: 12 (8 Claude Code, 3 Cursor, 1 manual)
- Files in folder: 47 (23 tracked, 24 ignored)
- Last 5 sessions:
  - 2026-04-28 19:30 — "fix the routing collision in M2 layer" (Claude Code)
  - 2026-04-28 14:12 — "reproduce fig 7 from the paper" (Claude Code)
  - ...

## Key files
- `charge.py` — TFLN charge solver, last edited in session abc123 (matched Fig 7)
- `plan.md` — current implementation plan, edited 4× across 3 sessions
- ...

## Open threads
- Session def456 ended with "TODO: try smaller mesh"
- ...

## How to use this handoff
- Read `CONTEXT.md` for an LLM-targeted intro.
- Inspect `files/` for the current folder state.
- See `sessions/` for full transcripts.
```

## Sync & sharing (post-v1, but design accommodates)

**Cloud backup**: bring-your-own-bucket. `lyre remote add backup
s3://my-bucket/lyrebird/myproject` then `lyre push backup`. Runs `git push`
under the hood with a remote helper. No hosted service from us — keeps the
project privacy-respecting and operationally light.

**Team sharing**: same mechanism. `git push` to a shared remote; teammates
`lyre clone`. v2 might add a "live channel" with conflict resolution.

**Privacy**: pre-sync scrubber checks for high-entropy strings and common
secret patterns; warns before push. Everything stays local by default.

## Install UX

```bash
curl -fsSL https://lyrebird.dev/install.sh | sh
```

This installer:
1. Drops the `lyre` static binary at `~/.local/bin/lyre`
2. Installs a launchd plist (macOS) or systemd unit (Linux) for the watcher daemon
3. Prompts to register the Claude Code `PostToolUse` hook in `~/.claude/settings.json` (with confirmation)
4. Prints next steps

Per-folder activation:
```bash
cd ~/Documents/LocalRepos/MyNextProject
lyre init   # Creates .lyrebird/, registers folder, daemon picks it up
```

Auto-attach (opt-in power-user mode):
```bash
lyre auto on --root ~/Documents/LocalRepos
# Daemon watches the root; auto-init on any folder where an AI agent first writes
```

## Implementation choices

- **Language**: Go. Single static binary, fast, easy cross-compile, great `fsnotify` and embedded SQLite (`modernc.org/sqlite`) story. Rust is also fine; Go ships faster.
- **Web UI**: server-side rendered Go + HTMX. No SPA build step. Embed assets in the binary.
- **Git invocation**: shell out to system `git` (universal) rather than embedding `libgit2`. Simpler.
- **Daemon process**: launchd / systemd-managed; single process per user; talks to CLI over a unix socket at `~/.lyre/daemon.sock`.

## Build order

| Phase | Scope | Time est. |
|---|---|---|
| **1. CLI skeleton** | `lyre init`, `lyre snapshot <file>` (manual), `lyre log`, `lyre show`, `lyre restore`. Pure git wrapper. | 1-2 days |
| **2. Watcher daemon** | Auto-snapshot on file change. Works for manual edits. macOS only initially. | 2 days |
| **3. Claude Code adapter** | `PostToolUse` hook. Sessions/turns indexed. | 1 day |
| **4. Codex CLI adapter** | Tail session logs. | 1 day |
| **5. Web UI (timeline + session + file views)** | The "smell test" demo. | 3-4 days |
| **6. Handoff packager** | `lyre handoff` produces the deliverable. | 2 days |
| **7. Revert + fork polish** | UX + safety guarantees. | 1 day |
| **8. Cursor adapter** | SQLite tail. | 2 days |
| **9. VS Code extension** | For Continue/Cline/Copilot Chat capture. | 3-4 days |
| **10. Cloud sync (BYO)** | Wire up `git push` to S3 with credentials helper. | 2 days |

**MVP target = phases 1-6**, ~10-12 working days. That gets you the smell-test
workflow: run it in your next project, open `lyre ui`, hand a folder + summary
to another AI.

## Open questions (last few)

1. **Domain / repo name**. Want me to check `lyrebird.dev`, `lyrebird.io`, etc., or pick later?
2. **Org/repo location**. GitHub `prash-flexcompute/lyrebird`, a new org (`lyrebird-dev`?), or somewhere else?
3. **License**. MIT? Apache 2.0? AGPL (in case you ever want to commercialize the hosted version)?
4. **Telemetry**. Should the daemon phone home anonymously (just a count of installs)? Default off, opt-in? My instinct: zero telemetry in v1.
