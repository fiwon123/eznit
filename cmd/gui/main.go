package main

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

func main() {
	port := "8765"
	http.Handle("/", http.FileServer(http.Dir("web/dist")))

	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser("http://localhost:" + port)
	}()

	fmt.Printf("eznit GUI running at http://localhost:%s\n", port)
	fmt.Println("Press Ctrl+C to quit")
	http.ListenAndServe(":"+port, nil)
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
