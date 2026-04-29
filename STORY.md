# Lyrebird — the story so far

A casual narrative of how Lyrebird came to exist and what it does today.
For the technical log, see [JOURNAL.md](./JOURNAL.md). For the spec, see
[DESIGN.md](./DESIGN.md). For the original problem statement, see
[PROBLEM.md](./PROBLEM.md).

---

## The problem

> "As I work with AI on simulations, my local folders have lots and lots of
> files. Sometimes I have a hard time knowing where things are and also
> reverting to a previous stage… I don't have a sense of checkpointing and
> version control."

Real evidence from one folder: eight near-identical `LNOI400_*_demo*.ipynb`
files (`_demo`, `_demo_notebook`, `_demo_final`, …) — classic "AI made a
copy because nobody trusts in-place edits." Plus scratch files at the
project root, in-place overwrites of generated PNGs, and the *reasoning*
behind every change living only in chat threads scattered across Claude
Code and Cursor sessions.

Git solves the file-versioning half. It doesn't solve the chat-attribution
half. And it's too manual for an exploratory AI workflow — you don't want
to be authoring commit messages while the AI is editing files for you.

## The brainstorm

We sketched a tool that:
- **Silently** snapshots every file change (no commits to author)
- **Captures the chat thread** that caused each change, alongside it
- **Stays local-first**, with optional sync later
- **Installs in one command** and "just runs"

Brainstormed names from an animal theme — settled on **Lyrebird** because the
real bird is famous for *perfectly recording and replaying any sound it
hears*. The CLI is `lyre`.

## The design

Three layers, each with a chosen mechanism:

- **Capture**: hybrid — a filesystem watcher catches every write, *plus*
  Claude Code's `PostToolUse` hook fires alongside to enrich AI-driven
  changes with chat-thread metadata.
- **Storage**: git, under the hood. Hidden repo at `.lyrebird/git/` with
  `GIT_DIR` + `GIT_WORK_TREE` set so it never collides with any user-owned
  `.git/` in the same folder.
- **Surface**: a CLI, plus a local web UI, plus a `handoff` command that
  packages a folder + summary for handoff to another AI.

Decision rationale for the major calls in [BRAINSTORM.md](./BRAINSTORM.md)
and [BRAINSTORM_VISUAL.md](./BRAINSTORM_VISUAL.md).

## The build, in seven phases

1. **CLI skeleton + git wrapper** (`lyre init`, `snapshot`, `log`, `show`,
   `restore`, `revert`). All commands shell out to git with the hidden-repo
   env vars.
2. **Filesystem watcher** (`lyre watch`). FSEvents-based, debounces 2s
   after the last write, recursively adds new directories. Filters editor
   temp files, swap files, and Lyrebird's own dir.
3. **Claude Code adapter** (`lyre hook` + `lyre install-hook`). Reads the
   `PostToolUse` JSON payload from stdin, finds the lyrebird repo, takes
   a snapshot, and persists session metadata. Tails the agent's transcript
   file to grab the user prompt + assistant text for each turn.
4. **Handoff packager** (`lyre handoff`). Produces `HANDOFF.md` (deterministic
   summary, no LLM call), `CONTEXT.md` (LLM-targeted intro), `files/` (current
   folder state via `git archive`), `sessions/` (full transcripts),
   `timeline.json` (machine-readable event log).
5. **Web UI** (`lyre ui`). Embedded HTTP server on port 6789, server-rendered
   HTML with Lucide icons, light/dark theme toggle, full-text search.
6. **End-to-end test**. Scripted scenario: init → manual edits → simulated
   AI sessions → restore → handoff → UI inspection. Every piece works.

## The UX overhaul

The first UI was a git frontend dressed up — words like *snapshot*, *hash*,
*diff*, *manual* meant nothing to a non-developer. After feedback ("this UI
is very opaque to an average user"), I rewrote the home page around the
user's mental model:

- **Vocabulary swap**: snapshot→change, hash→id, diff→"what changed",
  revert→undo, restore→"bring back this version", session→conversation,
  manual→You (✋ avatar), claude-code→Claude (🤖), system events→Lyrebird (🐦)
- **Story view**: events bucketed by day with chapter cards. Each day is
  "Chapter N · Yesterday" with a serif heading. Within each chapter, a
  vertical spine with actor-colored nodes — modern and book-like at once.
- **GitHub-style colored diff** on the snapshot detail page (no more raw
  monospace dump).
- **Folder tree** in the timeline hero — collapsible, properly nested,
  scales gracefully when there are many files.
- **Time-travel page** — drag a slider to scrub through history; the
  folder's file tree updates dynamically to show what existed at that
  moment. One-click "Bring the folder back to this state."
- **Restore reachable from every page** — timeline events, snapshot
  detail, file history, time travel, hero "Undo" button. All five paths
  auto-snapshot first so any rewind is reversible.

## What it looks like today

The home page reads as a story:

```
lyrebird_test
3 from Claude · 6 from you · across 2 conversations.

[Undo]  [Save now]  [Travel]  [Hand off]

📁 6 files in this folder ▾
   ▸ transcripts/
       · sess1.jsonl
       · sess2.jsonl
   · README.md  · fib.py  · scratch.md  · test_fib.py

CHAPTER 2 · Today
  ●━━ 🐦 Lyrebird   Rolled the folder back to an earlier state · 17m ago
  ●━━ ✋ You        edited scratch.md · 23m ago

CHAPTER 1 · Yesterday
  ●━━ 🤖 Claude     edited fib.py and transcripts/sess1.jsonl · 7h ago
       "Add a main block that prints first 10"
       ↺ show what changed   ↶ rewind to here
  …
```

Every event is clickable. The whole row tints on hover. The diff page
opens to a GitHub-style red/green view with the chat thread that caused
the change. Time travel lets you scrub back to any moment and see the
exact tree of files that existed then.

## What's left

Deferred for later (logged in JOURNAL.md):

- One-curl install with prebuilt binary (this is the next thing — see below)
- Codex CLI adapter (tail `~/.codex/sessions/*.jsonl`)
- Cursor adapter (tail SQLite `state.vscdb`)
- VS Code extension for Continue / Cline / Copilot Chat capture
- Cloud sync (`lyre push s3://my-bucket`)
- Pre-sync secret scrubber
- Linux + Windows builds
- launchd plist so the watcher daemon survives reboot

## Numbers

| | |
|---|---|
| Total commits on `main` | ~14 |
| Lines of Go | ~3,400 |
| External Go deps | 1 (`fsnotify`) |
| Build time | ~3s |
| Binary size | ~8.4 MB |
| MVP scope | phases 1–6 from DESIGN.md |
| Days from blank repo to working MVP | 1 |
