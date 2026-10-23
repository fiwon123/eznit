// The GUI is the web UI in a window: it serves the copy embedded in the
// binary and opens a browser on it.
//
// The pages call /v1 on their own origin, so this process has to answer those
// calls too. It does that in one of two ways (#60):
//
//   - GUI_API_URL names a running API, and /v1 is proxied to it;
//   - otherwise the DB_* variables describe a database, and the API runs in
//     this process on the same listener.
//
// With neither, the UI still opens and every /v1 call answers 503 with a
// message that says which variable to set — a configurable failure instead of
// a static file server's 404.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/fiwon123/eznit/internal/platform/api"
	"github.com/fiwon123/eznit/internal/platform/sql"
	"github.com/fiwon123/eznit/internal/platform/webui"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/fiwon123/eznit/web"
	"github.com/joho/godotenv"
)

const (
	// defaultPort is where the UI and the API answer. GUI_PORT moves it.
	defaultPort = "8765"
	// distFolder is the development copy of the web UI, relative to the
	// working directory. A released GUI carries its own copy instead.
	distFolder = "web/dist"
)

func main() {
	// The GUI reads the same files as the API: one database configuration,
	// whichever binary is started.
	_ = godotenv.Load(".env.local", ".env")

	logger, err := logger.NewJson(envOr("GUI_LOGS", "./logs/"), false)
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Sync()

	ui, uiSource := uiFileSystem()
	if ui == nil {
		fmt.Fprintln(os.Stderr, "No web UI to show: none is embedded in this build and "+distFolder+" is not there either.")
		fmt.Fprintln(os.Stderr, "Run `make web_build` and then rebuild the GUI (`make build_gui`).")
		os.Exit(1)
	}

	apiHandler, apiSource := whatAnswersV1(logger)

	http.Handle("/", webui.Handler(ui, apiHandler))

	port := envOr("GUI_PORT", defaultPort)
	address := "http://localhost:" + port

	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(address)
	}()

	fmt.Printf("eznit GUI running at %s\n", address)
	fmt.Printf("Serving the web UI from %s\n", uiSource)
	fmt.Printf("Serving /v1 from %s\n", apiSource)
	fmt.Println("Press Ctrl+C to quit")

	err = http.ListenAndServe(":"+port, nil)
	if !errors.Is(err, http.ErrServerClosed) {
		fmt.Println(err)
	}
}

// whatAnswersV1 picks the handler for /v1, and describes it for the log.
//
// The order matters: a GUI given an API URL is a launcher and must not open a
// second connection to the database behind it. Only when no API is named does
// this process become the API, and only when a database is configured.
func whatAnswersV1(logger *logger.Config) (http.Handler, string) {
	if target := os.Getenv("GUI_API_URL"); target != "" {
		proxy, err := proxyAPI(target, logger)
		if err != nil {
			logger.Error("GUI_API_URL is not usable", slog.String("url", target), slog.Any("error", err))
			return unavailableHandler("GUI_API_URL is not a usable URL: " + err.Error() + "."),
				"nothing: GUI_API_URL is invalid"
		}

		return proxy, "the API at " + target
	}

	if os.Getenv("DB_NAME") == "" {
		message := "No API is configured: set GUI_API_URL to a running API, or the DB_* variables (with API_UPLOADS) so this window can run the API itself. See ARCHITECTURE.md."
		logger.Warn(message)

		return unavailableHandler(message), "nothing: no API configured"
	}

	db, err := sql.Open("postgres", sql.DSNFromEnv())
	if err != nil {
		message := "The database is configured but this window cannot reach it: " + err.Error() + ". Start the database, or set GUI_API_URL to a running API."
		logger.Error("the GUI cannot reach the database", slog.Any("error", err))

		return unavailableHandler(message), "nothing: the database is unreachable"
	}

	return api.NewRouter(api.Config{
			DB:           db,
			Logger:       logger,
			UploadFolder: os.Getenv("API_UPLOADS"),
		}),
		"the API in this process, backed by PostgreSQL at " + os.Getenv("DB_HOST")
}

// proxyAPI forwards /v1 to a separate API.
//
// A request that fails does not leave the page with a bare connection error:
// the proxy answers 503 with a message naming the target, which is the one
// piece of information the person in front of the window needs.
func proxyAPI(target string, logger *logger.Config) (http.Handler, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	if targetURL.Scheme == "" || targetURL.Host == "" {
		return nil, fmt.Errorf("%q is not an absolute URL", target)
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("the API is not reachable", slog.String("url", target), slog.Any("error", err))
		unavailableHandler("The API at "+target+" is not reachable: "+err.Error()+". Start it, or point GUI_API_URL at the right address.").ServeHTTP(w, r)
	}

	return proxy, nil
}

// unavailableHandler answers a /v1 request with the reason there is no API
// behind it.
func unavailableHandler(message string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		helper.SendErrorJson(w, http.StatusServiceUnavailable, message)
	})
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

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func openBrowser(address string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", address).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", address).Start()
	case "darwin":
		err = exec.Command("open", address).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Println("Please open", address, "in your browser")
	}
}
