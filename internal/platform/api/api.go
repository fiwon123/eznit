// Package api builds the HTTP API: the route table, the handlers and the
// session guard, wired to a database.
//
// It is a package rather than part of cmd/api because two processes serve the
// same API: the API binary, and the GUI, which runs the API in its own process
// when it is given a database and no separate API to talk to (#60). One
// router means the two cannot drift apart.
package api

import (
	"net/http"

	"github.com/fiwon123/eznit/internal/domain/commands"
	"github.com/fiwon123/eznit/internal/domain/files"
	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/domain/users"
	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/fiwon123/eznit/internal/platform/openapi"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// Config is what the router needs: a database to read and write, a logger, and
// the folder uploaded bytes are stored under.
type Config struct {
	DB           *sqlx.DB
	Logger       *logger.Config
	UploadFolder string
}

// NewRouter returns the API's router, with every /v1 route mounted on it.
func NewRouter(cfg Config) *chi.Mux {
	r := chi.NewRouter()

	r.Get("/v1/healthcheck", healthcheck)

	// The contract and the UI over it. Public on purpose: a client author
	// reads them before they have a token.
	openapi.Mount(r)

	sessionsRepo := sessions.NewRepository(cfg.DB, cfg.Logger)
	sessionsService := sessions.NewService(sessionsRepo, cfg.Logger)

	guard := middleware.NewGuard(sessionsService, cfg.Logger)

	userRepo := users.NewRepository(cfg.DB, cfg.Logger)
	userService := users.NewService(userRepo, sessionsService, cfg.Logger)
	userHandler := users.NewHandler(userService, sessionsService, guard, cfg.Logger)
	userHandler.RegisterRoutes(r)

	fileRepo := files.NewRepository(cfg.DB, cfg.Logger)
	fileService := files.NewService(fileRepo, cfg.UploadFolder, cfg.Logger)
	fileHandler := files.NewHandler(fileService, guard, cfg.Logger)
	fileHandler.RegisterRoutes(r)

	templateRepo := templates.NewRepository(cfg.DB, cfg.Logger)
	templateService := templates.NewService(templateRepo, cfg.UploadFolder, cfg.Logger)
	templateHandler := templates.NewHandler(templateService, guard, cfg.Logger)
	templateHandler.RegisterRoutes(r)

	commandRepo := commands.NewRepository(cfg.DB, cfg.Logger)
	commandService := commands.NewService(commandRepo, cfg.UploadFolder, cfg.Logger)
	commandHandler := commands.NewHandler(commandService, guard, cfg.Logger)
	commandHandler.RegisterRoutes(r)

	return r
}

// Answer the health check without touching the database: it reports that the
// process is up, which is what a container health check asks.
func healthcheck(w http.ResponseWriter, _ *http.Request) {
	helper.SendMessageJson(w, http.StatusOK, "available")
}
