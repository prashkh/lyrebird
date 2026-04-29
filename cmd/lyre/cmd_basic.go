// Phase 1 commands: init, status, snapshot, log, show, restore, revert.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prashkh/lyrebird/internal/config"
	"github.com/prashkh/lyrebird/internal/gitstore"
	"github.com/prashkh/lyrebird/internal/registry"
	"github.com/prashkh/lyrebird/internal/session"
)

func cmdInit(args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Idempotent: if already initialized, just refresh the registry entry
	// and tell the user. Don't error out — this is the obvious thing to
	// run after upgrading from a pre-v0.2.0 release where the registry
	// didn't exist yet.
	if existing, err := config.Open(cwd); err == nil {
		if reg, err := registry.LoadDefault(); err == nil {
			p, _ := reg.Register(existing.Config.FolderName, existing.Root)
			_ = reg.Save()
			fmt.Printf("Already tracking %s\n", existing.Root)
			fmt.Printf("Registered as project %q (id: %s)\n", p.Name, p.ID)
			fmt.Println("Run `lyre ui` to see all your tracked folders, or refresh the existing UI tab.")
		} else {
			fmt.Printf("Already tracking %s (registry unavailable: %v)\n", existing.Root, err)
		}
		return nil
	}

	cfg := config.Config{
		Version:    1,
		Created:    time.Now().UTC().Format(time.RFC3339),
		FolderName: filepath.Base(cwd),
	}
	repo, err := config.CreateAt(cwd, cfg)
	if err != nil {
		return err
	}
	gs := gitstore.New(repo)
	if err := gs.Init(); err != nil {
		return err
	}
	// Write default .lyreignore if not present.
	if _, err := os.Stat(repo.IgnorePath()); os.IsNotExist(err) {
		_ = os.WriteFile(repo.IgnorePath(), []byte(config.DefaultIgnoreContents), 0o644)
	}
	// Take an initial snapshot of the current state.
	hash, err := gs.Snapshot("[lyre] initial snapshot at lyre init")
	if err != nil {
		return fmt.Errorf("initial snapshot: %w", err)
	}
	// Register the folder in the global registry so `lyre ui` from
	// anywhere can list it as a project.
	if reg, err := registry.LoadDefault(); err == nil {
		if _, err := reg.Register(repo.Config.FolderName, repo.Root); err == nil {
			_ = reg.Save()
		}
	}

	fmt.Printf("Initialized lyrebird repo at %s\n", repo.LyrebirdDir)
	if hash != "" {
		fmt.Printf("Initial snapshot: %s\n", hash[:12])
	} else {
		fmt.Println("Initial snapshot: (empty folder)")
	}
	fmt.Println("Next steps:")
	fmt.Println("  lyre watch &        # auto-snapshot on every file change")
	fmt.Println("  lyre install-hook   # capture Claude Code chat threads")
	fmt.Println("  lyre ui             # open the timeline UI (shows ALL tracked folders)")
	return nil
}

// cmdRegister adds (or refreshes) the current folder in the global registry.
// Useful for folders that were tracked before the registry was introduced,
// or to rename a project.
func cmdRegister(args []string) error {
	repo, err := config.Open(".")
	if err != nil {
		return err
	}
	reg, err := registry.LoadDefault()
	if err != nil {
		return err
	}
	p, err := reg.Register(repo.Config.FolderName, repo.Root)
	if err != nil {
		return err
	}
	if err := reg.Save(); err != nil {
		return err
	}
	fmt.Printf("Registered %q (id: %s)\n", p.Name, p.ID)
	fmt.Printf("Visible in `lyre ui` as a project on the home page.\n")
	return nil
}

// cmdProjects lists every tracked folder.
func cmdProjects(args []string) error {
	reg, err := registry.LoadDefault()
	if err != nil {
		return err
	}
	all := reg.All()
	if len(all) == 0 {
		fmt.Println("(no folders tracked yet — run `lyre init` in one)")
		return nil
	}
	for _, p := range all {
		fmt.Printf("%-20s  %s\n", p.Name, p.Root)
	}
	return nil
}

func cmdStatus(args []string) error {
	repo, err := config.Open(".")
	if err != nil {
		return err
	}
	gs := gitstore.New(repo)
	head := gs.CurrentHead()
	fmt.Printf("Repo:    %s\n", repo.Root)
	fmt.Printf("Created: %s\n", repo.Config.Created)
	if head != "" {
		fmt.Printf("HEAD:    %s\n", head[:12])
	} else {
		fmt.Println("HEAD:    (no snapshots yet)")
	}
	ss := session.New(repo)
	list, err := ss.List()
	if err == nil {
		fmt.Printf("Sessions: %d\n", len(list))
	}
	return nil
}

func cmdSnapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	msg := fs.String("m", "", "snapshot message")
	_ = fs.Parse(args)

	repo, err := config.Open(".")
	if err != nil {
		return err
	}
	gs := gitstore.New(repo)
	message := *msg
	if message == "" {
		message = "[manual] " + time.Now().UTC().Format(time.RFC3339)
	} else {
		message = "[manual] " + message
	}
	hash, err := gs.Snapshot(message)
	if err != nil {
		return err
	}
	if hash == "" {
		fmt.Println("Nothing to snapshot (working tree clean).")
		return nil
	}
	fmt.Printf("Snapshot: %s\n", hash[:12])
	return nil
}

func cmdLog(args []string) error {
	fs := flag.NewFlagSet("log", flag.ExitOnError)
	n := fs.Int("n", 50, "max number of entries")
	_ = fs.Parse(args)

	repo, err := config.Open(".")
	if err != nil {
		return err
	}
	gs := gitstore.New(repo)
	ss := session.New(repo)
	entries, err := gs.Log(*n, fs.Args()...)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("(no snapshots yet)")
		return nil
	}
	for _, e := range entries {
		// Find chat-thread metadata for this commit.
		sess, turn, _ := ss.FindByCommit(e.Hash)
		fmt.Printf("%s  %s  %s\n", e.ShortHash, e.Date[:19], e.Subject)
		if sess != nil && turn != nil {
			prompt := turn.UserPrompt
			if len(prompt) > 100 {
				prompt = prompt[:100] + "..."
			}
			fmt.Printf("    └─ %s · turn %s · %s\n", sess.Agent, shortID(turn.TurnID), strings.ReplaceAll(prompt, "\n", " "))
		}
	}
	return nil
}

func cmdShow(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: lyre show <id>")
	}
	id := args[0]
	repo, err := config.Open(".")
	if err != nil {
		return err
	}
	gs := gitstore.New(repo)
	ss := session.New(repo)
	// Resolve short hash to full hash for the session lookup.
	full, err := gs.Log(1, "--", id) // not ideal; use rev-parse via show
	_ = full
	out, err := gs.Show(id)
	if err != nil {
		return err
	}
	fmt.Println(out)
	// Try to find chat metadata by full hash.
	if entries, _ := gs.Log(1); len(entries) > 0 {
		for _, e := range entries {
			if strings.HasPrefix(e.Hash, id) {
				sess, turn, _ := ss.FindByCommit(e.Hash)
				if sess != nil && turn != nil {
					fmt.Println("\n━━━ Chat thread ━━━")
					fmt.Printf("Session: %s (%s)\n", sess.SessionID, sess.Agent)
					fmt.Printf("Turn:    %s\n", turn.TurnID)
					if turn.UserPrompt != "" {
						fmt.Printf("\nUser:\n%s\n", turn.UserPrompt)
					}
					if turn.AssistantText != "" {
						fmt.Printf("\nAssistant:\n%s\n", turn.AssistantText)
					}
				}
				break
			}
		}
	}
	return nil
}

func cmdRestore(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: lyre restore <file> <id>")
	}
	file := args[0]
	id := args[1]
	repo, err := config.Open(".")
	if err != nil {
		return err
	}
	gs := gitstore.New(repo)
	// Safety snapshot first.
	if _, err := gs.Snapshot("[safety] before restore of " + file + " from " + id); err != nil {
		return fmt.Errorf("safety snapshot: %w", err)
	}
	if err := gs.Restore(id, file); err != nil {
		return err
	}
	fmt.Printf("Restored %s from %s. (Previous state saved.)\n", file, id)
	// Snapshot the restore itself so it's part of history.
	_, _ = gs.Snapshot("[restore] " + file + " from " + id)
	return nil
}

func cmdRevert(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: lyre revert <id>")
	}
	id := args[0]
	repo, err := config.Open(".")
	if err != nil {
		return err
	}
	gs := gitstore.New(repo)
	if _, err := gs.Snapshot("[safety] before revert to " + id); err != nil {
		return fmt.Errorf("safety snapshot: %w", err)
	}
	if err := gs.Revert(id); err != nil {
		return err
	}
	_, _ = gs.Snapshot("[revert] folder reverted to " + id)
	fmt.Printf("Folder reverted to %s. (Previous state saved.)\n", id)
	return nil
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
