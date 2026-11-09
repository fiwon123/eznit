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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fiwon123/eznit/internal/domain/commands"
	"github.com/fiwon123/eznit/internal/domain/files"
	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/domain/users"
	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/fiwon123/eznit/internal/platform/openapi"
	"github.com/fiwon123/eznit/internal/platform/ratelimit"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// Config is what the router needs: a database to read and write, a logger, the
// folder uploaded bytes are stored under, and the auth rate-limit settings.
type Config struct {
	DB           *sqlx.DB
	Logger       *logger.Config
	UploadFolder string
	RateLimit    RateLimitConfig
}

// RateLimitConfig is the auth throttling the router enforces. Build it with
// RateLimitFromEnv so every binary reads the same variables.
type RateLimitConfig struct {
	Enabled        bool
	TrustProxy     bool
	Window         time.Duration
	LoginPerIP     int
	LoginPerEmail  int
	SignupPerIP    int
	SignupPerEmail int
}

// RateLimitFromEnv reads the auth rate-limit settings. Every value has a safe
// default, so an unset environment throttles too rather than not at all.
func RateLimitFromEnv() RateLimitConfig {
	return RateLimitConfig{
		Enabled:        envBool("RATE_LIMIT_ENABLED", true),
		TrustProxy:     envBool("TRUST_PROXY", false),
		Window:         envDuration("RATE_LIMIT_WINDOW", 15*time.Minute),
		LoginPerIP:     envInt("RATE_LIMIT_LOGIN_IP", 20),
		LoginPerEmail:  envInt("RATE_LIMIT_LOGIN_EMAIL", 10),
		SignupPerIP:    envInt("RATE_LIMIT_SIGNUP_IP", 10),
		SignupPerEmail: envInt("RATE_LIMIT_SIGNUP_EMAIL", 5),
	}
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

	userRepo := users.NewRepository(cfg.DB, cfg.Logger)
	userService := users.NewService(userRepo, cfg.UploadFolder, sessionsService, cfg.Logger)

	// The guard asks the users service who is an admin. It is built after the
	// service for that reason, and the service is passed as the AdminChecker
	// interface the guard declares — neither package imports the other.
	guard := middleware.NewGuard(sessionsService, userService, cfg.Logger)

	var limiter *ratelimit.Limiter
	if cfg.RateLimit.Enabled {
		limiter = ratelimit.New(cfg.RateLimit.Window)
	}

	userHandler := users.NewHandler(userService, sessionsService, guard, cfg.Logger, users.RateLimitPolicy{
		Limiter:        limiter,
		TrustProxy:     cfg.RateLimit.TrustProxy,
		LoginPerIP:     cfg.RateLimit.LoginPerIP,
		LoginPerEmail:  cfg.RateLimit.LoginPerEmail,
		SignupPerIP:    cfg.RateLimit.SignupPerIP,
		SignupPerEmail: cfg.RateLimit.SignupPerEmail,
	})
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

// envBool reads a boolean flag. A quoted value (as `.env` may hold) is accepted;
// anything unrecognised falls back to the default rather than guessing.
func envBool(name string, def bool) bool {
	switch strings.ToLower(envValue(name)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func envDuration(name string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(envValue(name)); err == nil && d > 0 {
		return d
	}
	return def
}

func envInt(name string, def int) int {
	if n, err := strconv.Atoi(envValue(name)); err == nil {
		return n
	}
	return def
}

// envValue trims the spaces and the surrounding quotes a `.env` line may carry.
func envValue(name string) string {
	return strings.Trim(strings.TrimSpace(os.Getenv(name)), `"`)
}
