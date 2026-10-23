package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/fiwon123/eznit/internal/domain/files"
	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/domain/users"
	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/fiwon123/eznit/internal/platform/sql"
	"github.com/fiwon123/eznit/internal/platform/webui"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/fiwon123/eznit/web"
	"github.com/go-chi/chi/v5"
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

	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	pwd := getDBPassword()
	name := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable", user, pwd, host, port, name)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		logger.Error("Unable to connect to PostgreSQL!")
		log.Fatal(err)
	}
	defer db.Close()

	logger.Info("Connected to PostgreSQL!")

	r := chi.NewRouter()

	r.Get("/v1/healthcheck", healthcheckHandler)

	sessionsRepo := sessions.NewRepository(db, logger)
	sessionsService := sessions.NewService(sessionsRepo, logger)

	guard := middleware.NewGuard(sessionsService, logger)

	userRepo := users.NewRepository(db, logger)
	userService := users.NewService(userRepo, sessionsService, logger)
	userHandler := users.NewHandler(userService, sessionsService, guard, logger)
	userHandler.RegisterRoutes(r)

	uploadFolder := os.Getenv("API_UPLOADS")
	fmt.Println(uploadFolder)
	fileRepo := files.NewRepository(db, logger)
	fileService := files.NewService(fileRepo, uploadFolder, logger)
	fileHandler := files.NewHandler(fileService, guard, logger)
	fileHandler.RegisterRoutes(r)

	templateRepo := templates.NewRepository(db, logger)
	templateService := templates.NewService(templateRepo, logger)
	templateHandler := templates.NewHandler(templateService, guard, logger)
	templateHandler.RegisterRoutes(r)

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

func getDBPassword() string {
	if p := os.Getenv("DB_PASSWORD_FILE"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return os.Getenv("DB_PWD")
}

func healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	helper.SendMessageJson(w, http.StatusOK, "available")
}
