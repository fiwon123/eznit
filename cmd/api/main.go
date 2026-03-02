package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/fiwon123/eznit/internal/domain/files"
	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/internal/domain/users"
	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/fiwon123/eznit/internal/platform/sql"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
)

func main() {

	var debugFlag bool
	flag.BoolVar(&debugFlag, "debug", false, "show debug logs")
	flag.Parse()

	logger := logger.New(true, debugFlag)
	defer logger.Sync()

	// .env.local overwrite .env for development
	_ = godotenv.Load(".env.local", ".env")

	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	pwd := getDBPassword()
	name := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable", user, pwd, host, port, name)
	fmt.Println(dsn)
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
	logger.Info("Initialized routes!")

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", 4000),
		Handler: r,
	}

	logger.Info("Server Running...")
	err = srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		fmt.Println(err)
	}

	logger.Info("Server Stopped")
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
