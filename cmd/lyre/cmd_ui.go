package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/prashkh/lyrebird/internal/config"
	"github.com/prashkh/lyrebird/internal/registry"
	"github.com/prashkh/lyrebird/internal/ui"
)

func cmdUI(args []string) error {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	port := fs.Int("p", 6789, "port to listen on")
	noOpen := fs.Bool("no-open", false, "don't auto-open the browser")
	_ = fs.Parse(args)

	// Load (or create) the global registry. The UI is always multi-project
	// — its home page is a list of every tracked folder.
	reg, err := registry.LoadDefault()
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}

	// If we're sitting inside a tracked folder that the registry doesn't
	// know about (e.g. the folder was tracked before this feature), opt
	// it in silently so the user sees it on the home page.
	if repo, err := config.Open("."); err == nil {
		if reg.ByRoot(repo.Root) == nil {
			if _, err := reg.Register(repo.Config.FolderName, repo.Root); err == nil {
				_ = reg.Save()
			}
		}
	}

	srv, err := ui.New(reg)
	if err != nil {
		return err
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Port %d is busy, picking a free one...\n", *port)
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("listen 127.0.0.1: %w", err)
		}
	}
	addr = listener.Addr().String()
	url := fmt.Sprintf("http://%s/", addr)

	// Project-aware open URL: if we're inside a tracked folder, open
	// straight to that project. Otherwise open the home page.
	openURL := url
	if cwd, err := os.Getwd(); err == nil {
		if p := reg.ByRoot(cwd); p == nil {
			// Walk up to find a tracked parent.
			if repo, err := config.Open(cwd); err == nil {
				p = reg.ByRoot(repo.Root)
			}
			if p != nil {
				openURL = url + "p/" + p.ID + "/"
			}
		} else {
			openURL = url + "p/" + p.ID + "/"
		}
	}

	fmt.Printf("🐦 Lyrebird UI: %s\n", url)
	fmt.Printf("   tracked folders: %d\n", len(reg.Projects))
	fmt.Println("   Ctrl-C to stop")

	if !*noOpen {
		go func() {
			time.Sleep(200 * time.Millisecond)
			openBrowser(openURL)
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
