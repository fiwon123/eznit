package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

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
	var handler http.Handler = r
	if ui, source := pickWebUI(os.DirFS(webDistFolder()), web.FS(), r); ui != nil {
		logger.Info("Serving the web UI from " + source)
		handler = ui
	} else {
		logger.Info("No web UI found in " + webDistFolder() + " and none built in — serving the API only")
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", 4000),
		Handler: handler,
	}

	logger.Info("Server Running...")
	err = srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		fmt.Println(err)
	}

	logger.Info("Server Stopped")
}

// webDistFolder is where the built web UI lives. It is relative to the
// working directory of the API, and API_WEB_DIST moves it.
func webDistFolder() string {
	if dir := os.Getenv("API_WEB_DIST"); dir != "" {
		return dir
	}
	return "web/dist"
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
