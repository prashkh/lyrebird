// Lyrebird CLI entry point.
package main

import (
	"fmt"
	"os"
)

const helpText = `lyre — silent versioning for AI-assisted folders

Usage:
  lyre <command> [args]

Commands:
  init                 Initialize lyrebird tracking in the current folder
  status               Show repo status
  snapshot [-m msg]    Take a manual snapshot
  log [<file>]         Show snapshot history (newest first)
  show <id>            Show diff + chat thread for a snapshot
  sessions             List AI sessions in this repo
  session <id>         Show full transcript + files touched
  search <query>       Full-text search across chats and file contents
  restore <file> <id>  Restore one file to its state at <id> (auto-snapshots first)
  revert <id>          Revert the entire folder to <id> (auto-snapshots first)
  watch                Run the FS watcher in the foreground (snapshots on change)
  hook [...]           Receive a Claude Code PostToolUse hook event (read JSON on stdin)
  handoff [-o <dir>]   Package the folder + summary for handoff to another AI
  ui [-p <port>]       Open the local web UI
  install-hook         Install Claude Code hook into ~/.claude/settings.json
  version              Print version

Run 'lyre <command> --help' for command-specific help.
`

const version = "0.1.0-dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Print(helpText)
		os.Exit(0)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = cmdInit(args)
	case "status":
		err = cmdStatus(args)
	case "snapshot":
		err = cmdSnapshot(args)
	case "log":
		err = cmdLog(args)
	case "show":
		err = cmdShow(args)
	case "sessions":
		err = cmdSessions(args)
	case "session":
		err = cmdSession(args)
	case "search":
		err = cmdSearch(args)
	case "restore":
		err = cmdRestore(args)
	case "revert":
		err = cmdRevert(args)
	case "watch":
		err = cmdWatch(args)
	case "hook":
		err = cmdHook(args)
	case "handoff":
		err = cmdHandoff(args)
	case "ui":
		err = cmdUI(args)
	case "install-hook":
		err = cmdInstallHook(args)
	case "version", "-v", "--version":
		fmt.Println("lyre", version)
	case "help", "-h", "--help":
		fmt.Print(helpText)
	default:
		fmt.Fprintf(os.Stderr, "lyre: unknown command %q\n\n", cmd)
		fmt.Fprint(os.Stderr, helpText)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lyre:", err)
		os.Exit(1)
	}
}
