package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/prashkh/lyrebird/internal/config"
	"github.com/prashkh/lyrebird/internal/ui"
)

func cmdUI(args []string) error {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	port := fs.Int("p", 6789, "port to listen on")
	noOpen := fs.Bool("no-open", false, "don't auto-open the browser")
	_ = fs.Parse(args)

	repo, err := config.Open(".")
	if err != nil {
		return err
	}
	srv, err := ui.New(repo)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	url := fmt.Sprintf("http://%s/", addr)
	fmt.Printf("🐦 Lyrebird UI: %s\n", url)
	fmt.Printf("   serving %s\n", repo.Root)
	fmt.Println("   Ctrl-C to stop")

	if !*noOpen {
		go func() {
			time.Sleep(200 * time.Millisecond)
			openBrowser(url)
		}()
	}
	server := &http.Server{Handler: srv.Routes()}
	return server.Serve(listener)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
