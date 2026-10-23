package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/fiwon123/eznit/internal/platform/api"
	"github.com/fiwon123/eznit/internal/platform/sql"
	"github.com/fiwon123/eznit/internal/platform/webui"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/fiwon123/eznit/web"
	"github.com/joho/godotenv"
)

func main() {

	// .env.local overwrite .env for development
	_ = godotenv.Load(".env.local", ".env")

	var debugFlag bool
	flag.BoolVar(&debugFlag, "debug", false, "show debug logs")
	flag.Parse()

	logsFolder, _ := os.LookupEnv("API_LOGS")
	if logsFolder == "" {
		logsFolder = "./logs/"
	}
	logger, err := logger.NewJson(logsFolder, debugFlag)
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Sync()

	db, err := sql.Open("postgres", sql.DSNFromEnv())
	if err != nil {
		logger.Error("Unable to connect to PostgreSQL!")
		log.Fatal(err)
	}
	defer db.Close()

	logger.Info("Connected to PostgreSQL!")

	r := api.NewRouter(api.Config{
		DB:           db,
		Logger:       logger,
		UploadFolder: os.Getenv("API_UPLOADS"),
	})

	logger.Info("Initialized routes!")

	// The web UI, when there is one. In development the Vite dev server
	// proxies /v1 to this API; in production this process serves the UI from
	// web/dist, or from the copy embedded at build time. One origin either
	// way, so the API needs no CORS headers.
	//
	// A deployment with a separate web tier (the `web` service in
	// docker-compose.yml) turns this off: then :4000 is the API only, and the
	// browser has exactly one frontend origin to talk to.
	var handler http.Handler = r
	if !webUIEnabled() {
		logger.Info("API_SERVE_WEB is off — serving the API only; the web UI is a separate service")
	} else if ui, source := pickWebUI(os.DirFS(webDistFolder()), web.FS(), r); ui != nil {
		logger.Info("Serving the web UI from " + source)
		handler = ui
	} else {
		logger.Info("No web UI found in " + webDistFolder() + " and none built in — serving the API only")
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", apiPort()),
		Handler: handler,
	}

	logger.Info("Server Running...")
	err = srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		fmt.Println(err)
	}

	logger.Info("Server Stopped")
}

// apiPort is the port the API listens on.
//
// It defaults to 4000 so the CLI, the GUI and existing scripts keep working
// without configuration. API_PORT overrides it for a deployment that needs a
// different bind port.
func apiPort() int {
	value := strings.TrimSpace(os.Getenv("API_PORT"))
	if value == "" {
		return 4000
	}

	port, err := strconv.Atoi(value)
	if err != nil {
		return 4000
	}

	if port <= 0 || port > 65535 {
		return 4000
	}

	return port
}

// webDistFolder is where the built web UI lives. It is relative to the
// working directory of the API, and API_WEB_DIST moves it.
func webDistFolder() string {
	if dir := os.Getenv("API_WEB_DIST"); dir != "" {
		return dir
	}
	return "web/dist"
}

// webUIEnabled reports whether this process should serve the web UI.
//
// The default is on: the released single binary stays self-contained, and a
// developer running the API alone still gets the UI from web/dist. A Compose
// deployment with a separate `web` service sets API_SERVE_WEB=false, so :4000
// stays the API only — one port, one job.
//
// An unparsable value keeps the UI on rather than silently dropping it: a typo
// in a deployment variable must not turn a working single binary into an
// API-only one.
func webUIEnabled() bool {
	value := strings.TrimSpace(os.Getenv("API_SERVE_WEB"))
	if value == "" {
		return true
	}

	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return true
	}

	return enabled
}

// pickWebUI chooses the web UI to serve in front of the API, and describes it
// for the log. It returns a nil handler when there is no UI to serve.
//
// A built web/dist wins: that is the development loop, where `make web_build`
// is visible without recompiling. The copy embedded at build time is the
// fallback, and it is what makes a released binary self-contained.
func pickWebUI(disk, embedded fs.FS, api http.Handler) (http.Handler, string) {
	if webui.HasIndex(disk) {
		return webui.Handler(disk, api), webDistFolder()
	}
	if webui.HasIndex(embedded) {
		return webui.Handler(embedded, api), "the copy embedded at build time"
	}
	return nil, ""
}
