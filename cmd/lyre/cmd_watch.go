package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/prashkh/lyrebird/internal/config"
	"github.com/prashkh/lyrebird/internal/watcher"
)

func cmdWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	debounceMs := fs.Int("debounce", 2000, "idle ms after last write before snapshotting")
	_ = fs.Parse(args)

	repo, err := config.Open(".")
	if err != nil {
		return err
	}
	logger := log.New(os.Stdout, "[lyre.watch] ", log.LstdFlags)
	w := watcher.New(repo, time.Duration(*debounceMs)*time.Millisecond, logger)
	return w.Run()
}
