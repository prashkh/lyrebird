package main

import (
	"flag"
	"fmt"

	"github.com/prashkh/lyrebird/internal/config"
	"github.com/prashkh/lyrebird/internal/handoff"
)

func cmdHandoff(args []string) error {
	fs := flag.NewFlagSet("handoff", flag.ExitOnError)
	out := fs.String("o", "", "output directory (default: .lyrebird/handoffs/handoff-<timestamp>)")
	_ = fs.Parse(args)

	repo, err := config.Open(".")
	if err != nil {
		return err
	}
	dir, err := handoff.Package(repo, *out)
	if err != nil {
		return err
	}
	fmt.Printf("Handoff package created at:\n  %s\n\n", dir)
	fmt.Println("Contents:")
	fmt.Println("  HANDOFF.md     — human-readable summary")
	fmt.Println("  CONTEXT.md     — LLM-targeted intro")
	fmt.Println("  files/         — current state of tracked folder")
	fmt.Println("  sessions/      — full chat transcripts")
	fmt.Println("  timeline.json  — machine-readable event log")
	return nil
}
