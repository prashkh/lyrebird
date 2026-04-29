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
- UI: SPA or HTMX? → HTMX. No build step, embeddable in the binary.
