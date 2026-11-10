//go:build fyne

// Command eznit-gui is the desktop app: a native Fyne window over an eznit
// API. The same routes the CLI and the server's web UI use.
//
// It is its own entry point so a double-click opens the window with no
// command tree between them. --help and --version exit before the toolkit
// starts, which is what lets a headless runner smoke-test the binary.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/fiwon123/eznit/internal/fyneui"
)

// Version is set at build time with -X main.Version=<tag>.
var Version = "dev"

func main() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Usage: eznit-gui [flags]")
		fmt.Fprintln(flag.CommandLine.Output())
		fmt.Fprintln(flag.CommandLine.Output(), "A native window over an eznit API, and the control panel")
		fmt.Fprintln(flag.CommandLine.Output(), "for a local eznit-server.")
		fmt.Fprintln(flag.CommandLine.Output())
		fmt.Fprintln(flag.CommandLine.Output(), "Flags:")
		flag.PrintDefaults()
	}

	api := flag.String("api", "", "the eznit API base URL (default: the stored setting, then GUI_API_URL)")
	serverDir := flag.String("server-dir", "", "the folder holding eznit-server and its .env (default: the stored setting, then this binary's folder)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(Version)
		return
	}

	if err := fyneui.Run(Version, *api, *serverDir); err != nil {
		fmt.Fprintln(os.Stderr, "eznit-gui:", err)
		os.Exit(1)
	}
}
