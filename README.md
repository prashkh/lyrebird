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

## Quick start

```bash
# (eventually) curl -fsSL https://lyrebird.dev/install.sh | sh
# for now, build from source:
go build -o bin/lyre ./cmd/lyre

# initialize tracking in a folder
cd ~/myproject
~/path/to/lyre init

# start the watcher (snapshots every change automatically)
lyre watch &

# explore history
lyre log
lyre sessions
lyre show <snapshot-id>

# package the folder + a summary for another AI
lyre handoff
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
