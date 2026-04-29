# Lyrebird

> Silent versioning for AI-assisted folders. Records every file change and
> the chat thread that caused it, so you can rewind, explore, and hand off
> a folder to another AI with full context.

**Status: early MVP, pre-1.0.** See [DESIGN.md](./DESIGN.md) for the full spec
and [JOURNAL.md](./JOURNAL.md) for the build narrative.

## Why

When AI agents work autonomously in a folder over many sessions, the folder
accumulates files but loses *history*. Lyrebird silently snapshots the folder
on every change and links each snapshot to the chat thread that caused it —
so "what did this file look like last Tuesday, and which Claude session put
it there?" becomes a one-command answer.

See [PROBLEM.md](./PROBLEM.md) for the motivating examples.

## Install

**macOS** (Apple Silicon or Intel):

```bash
curl -fsSL https://raw.githubusercontent.com/prashkh/lyrebird/main/install.sh | sh
```

That's it. The installer:
1. Detects your CPU and downloads the right `lyre` binary from the latest
   GitHub release (no Go, no source clone, no compile).
2. Drops it in the first writable directory on your `$PATH` — preferring
   `~/.local/bin`, then `/opt/homebrew/bin`, then `/usr/local/bin`.
3. Optionally registers the Claude Code `PostToolUse` hook in
   `~/.claude/settings.json` so chat threads are captured automatically.

Once installed, `lyre` is available in any folder. Linux & Windows
support are coming.

**Build from source** (developers, or if the prebuilt binary doesn't work
for your platform):

```bash
git clone https://github.com/prashkh/lyrebird && cd lyrebird && ./install-from-source.sh
```

## Quick start

```bash
# Initialize tracking in any folder
cd ~/myproject
lyre init

# Start the watcher in another terminal (snapshots every change)
lyre watch &

# Explore history
lyre log
lyre sessions
lyre session <id>
lyre show <snapshot-id>
lyre search "route_manhattan"

# Restore (always reversible — auto-snapshots current state first)
lyre restore <file> <snapshot-id>
lyre revert <snapshot-id>

# Open the timeline + session UI
lyre ui                      # http://localhost:6789

# Package the folder + a summary for another AI
lyre handoff -o ~/handoff
```

## Layout

```
.lyrebird/
├── git/        ← hidden git repo (the actual object store)
├── sessions/   ← per-AI-session JSON: prompts, transcripts
└── config.toml
```

The user's own `.git/` (if any) is never touched.

## License

MIT — see [LICENSE](./LICENSE).
