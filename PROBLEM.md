# Silent VCS for AI-Assisted Folders

> Working title — `silent-vcs` is a placeholder. Rename later.

## The problem in one sentence

When an AI agent works autonomously in a folder over many sessions, the folder
accumulates files but loses **history**: I can't tell what was written when, why,
or which chat thread produced it — and I can't roll back to "the version that
worked last Tuesday."

## What I'm actually seeing in my own folders

These are real examples from `~/Documents/LocalRepos/`, all produced by AI
agents (Claude Code, Cursor) working across many sessions:

### 1. Variant creep — the "just-in-case copy" pattern

`Tidy3d_tutorials/Tutorials/Luxtelligence/` contains:

```
LNOI400_birefringence_demo_notebook.ipynb
LNOI400_component_demo.ipynb
LNOI400_component_demo_final.ipynb
LNOI400_component_demo_notebook.ipynb
LNOI400_tech_demo.ipynb
LNOI400_tech_demo_notebook.ipynb
LNOI_400_tech_demo.ipynb
LNOI_component_demo.ipynb
```

Eight notebooks that are 80% the same. Each was created in a session where the
AI (or I) didn't trust an in-place edit and made a "_final", "_notebook",
"_demo" copy instead. None of them have a clear lineage. I cannot tell which
one is the canonical version, and none of them carry the conversation that
explains why they exist.

### 2. Top-level scratch detritus

The root of `Tidy3d_tutorials/` has:

```
scratch.ipynb
live_layout.py
live_preview.py
mode_solver.hdf5
mode_solver_batch_results_0.hdf5  ... mode_solver_batch_results_6.hdf5
```

These are mid-session artifacts. `live_layout.py` was the LiveViewer script
from some earlier task — but which task? When? If I want to recover the exact
state from when it last worked, I have to grep my chat history.

### 3. In-place overwrites of generated artifacts

`Tidy3d_tutorials/Tutorials/Segmented_MZM/02_charge/` contains:

```
plan.md
metrics.json
fig6_carrier_maps.png
fig7_reproduction.png
preflight_geometry.png
charge.py
charge_demo.ipynb
```

`plan.md` and `metrics.json` get rewritten each session. The PNGs get
regenerated. There's no record of "the metrics.json from when fig7 first
matched the paper" — only the most recent version survives.

### 4. The chat thread is the missing column

For every one of these files, the *real* explanation lives in a chat thread:
- which paper figure I was reproducing
- what convergence values we tried and discarded
- why we picked this geometry over the previous one
- what error we hit and how we worked around it

The file system stores the *result*. The chat stores the *reasoning*. They
live in totally separate systems and **the link between them is in my head**.
When my head forgets, the reasoning is gone.

## What specifically goes wrong

1. **Can't revert.** "Go back to the version that worked before today's edits"
   is a 30-minute archaeology session, not a one-command operation.
2. **Can't attribute.** "Where did this file come from?" has no answer. I
   can't tell which Claude Code session produced `live_layout.py`.
3. **Can't search history by intent.** "Find the script from the session
   where we got the JQ MRM heat-meshing fix working" — no way to query that.
4. **Can't trust in-place edits.** Because the AI (and I) don't trust that
   we can recover the previous version, we make `_final`, `_v2`, `_new`
   copies. This causes the variant explosion above.
5. **Can't share lineage.** If I send `LNOI400_component_demo_final.ipynb`
   to a colleague, they get a file with no history of how it got that way.

## Why the obvious tools don't solve this

| Tool | Why it falls short |
|------|---|
| **Git** | Manual. Requires `git init`, staging, commits, branches. AI agents don't reliably commit at the right moments, and asking the user to commit defeats the "silent" requirement. Also doesn't capture the chat thread. |
| **Jupyter `.ipynb_checkpoints`** | Only `.ipynb`, only most recent, no chat link. |
| **Time Machine / file-system snapshots** | Coarse-grained, no per-file diff workflow, no chat link, no semantic markers ("before vs after the routing fix"). |
| **Cursor / Claude Code session logs** | Chat lives in the IDE's app data, not in the folder. Folder is portable; chat isn't. If I `zip` and send the folder, the reasoning is left behind. |
| **Manual discipline** | Already failed — see the eight LNOI400 notebooks. |

## What I actually want

A tool with these properties:

1. **Zero-friction install.** One `curl | sh` command. No package manager
   dance, no Python venv, no Docker.
2. **Zero-friction activation.** Either `silent-vcs init` in a folder, or —
   ideally — it auto-detects folders that an AI agent is writing into and
   starts tracking them silently.
3. **Silent operation.** No prompts, no commits to author, no branches to
   manage. Just runs.
4. **History stored in-folder.** Whatever it captures lives next to the
   files (e.g. `.silent-vcs/`), so zipping the folder takes the history
   with it.
5. **Chat thread captured alongside files.** When Claude Code (or Cursor,
   or any agent) edits a file in the folder, the conversation that
   produced the edit is stored too, linked to the file change.
6. **Queryable.** "Show me the state of `charge.py` two sessions ago" or
   "which session produced this PNG" should be one command.
7. **Revertable.** "Roll the folder back to the state at the end of the
   2026-04-15 session" should be one command.

## What's deliberately out of scope (for now)

- Multi-user collaboration / sync (this is a local single-user tool).
- Replacing Git for "real" projects. This is for *exploratory* AI work,
  not production code.
- Pretty UI. CLI-first is fine.
- Cloud backup.

## Open questions to resolve next

These need answers before designing a solution. Not answering them now —
just noting them so they're not forgotten:

1. **Auto-detection mechanism.** How does the daemon know an AI agent is
   writing? Watch for Claude Code / Cursor processes? `fswatch` everywhere
   under `~/Documents`? Hook into the agent itself via a Claude Code hook?
2. **Chat capture mechanism.** Claude Code stores transcripts in
   `~/.claude/projects/<encoded-path>/`. Cursor stores them elsewhere. Do
   we tap those, or do we expect agents to push transcripts in via a hook?
3. **Storage format.** Per-file content-addressed blobs (Git-like)?
   Append-only event log? SQLite? The choice affects how cheaply we can
   answer "what did file X look like at session Y."
4. **Granularity of a "checkpoint."** Per-file-write? Per-tool-call?
   Per-session? Per-user-prompt?
5. **What about large binaries** (`.hdf5`, `.gds`, generated PNGs)?
   Storing every version of a 7 MB hdf5 explodes the history. Dedup?
   LFS-like external store? Skip-listed by default?
6. **Notebook handling.** `.ipynb` diffs are notoriously bad. Do we store
   them as-is or normalize (strip outputs, jupytext-ify) before hashing?
7. **Privacy.** Chat transcripts may contain API keys, file paths,
   business context. Anything in `.silent-vcs/` should be local-only by
   default and easy to scrub before sharing the folder.

## Success criteria

I'll know this works when:

- I install it once and forget it exists.
- Three weeks from now, I can run one command in a messy AI-edited folder
  and answer: "what did this file look like a week ago, and which Claude
  session put it there?"
- The Luxtelligence folder stops accumulating `_final` / `_v2` / `_notebook`
  copies, because the AI (and I) trust that history is safe.
