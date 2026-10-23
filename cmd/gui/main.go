package main

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/fiwon123/eznit/internal/platform/webui"
	"github.com/fiwon123/eznit/web"
)

const (
	port = "8765"
	// distFolder is the development copy of the web UI, relative to the
	// working directory. A released GUI carries its own copy instead.
	distFolder = "web/dist"
)

func main() {
	ui, source := uiFileSystem()
	if ui == nil {
		fmt.Fprintln(os.Stderr, "No web UI to show: none is embedded in this build and "+distFolder+" is not there either.")
		fmt.Fprintln(os.Stderr, "Run `make web_build` and then rebuild the GUI (`make build_gui`).")
		os.Exit(1)
	}

	// The GUI has no API of its own yet, and the pages call /v1 on this same
	// origin, so those requests answer 404 here. Issue #60 tracks wiring the
	// GUI to the API; until then this serves the UI and nothing else.
	http.Handle("/", webui.Handler(ui, http.NotFoundHandler()))

	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser("http://localhost:" + port)
	}()

	fmt.Printf("eznit GUI running at http://localhost:%s\n", port)
	fmt.Printf("Serving the web UI from %s\n", source)
	fmt.Println("Press Ctrl+C to quit")
	_ = http.ListenAndServe(":"+port, nil)
}

// uiFileSystem is the web UI the window shows: the copy embedded at build
// time, which is what makes the GUI one file. A developer who built web/dist
// but did not rebuild the GUI gets the disk copy instead.
func uiFileSystem() (fs.FS, string) {
	if ui := web.FS(); webui.HasIndex(ui) {
		return ui, "the copy embedded at build time"
	}
	if ui := os.DirFS(distFolder); webui.HasIndex(ui) {
		return ui, distFolder
	}
	return nil, ""
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Println("Please open", url, "in your browser")
	}
}
