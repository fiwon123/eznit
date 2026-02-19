package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/fiwon123/eznit/internal/domain/files"
	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/internal/domain/users"
	"github.com/fiwon123/eznit/internal/platform/sql"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/types"
	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
)

func main() {

	// local
	_ = godotenv.Load()

	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	pwd := os.Getenv("DB_PWD")
	name := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf("postgresql://%s:%s@localhost:%s/%s?sslmode=disable", user, pwd, port, name)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	r := chi.NewRouter()

	r.Get("/v1/healthcheck", healthcheckHandler)

	sessionsRepo := sessions.NewRepository(db)
	sessionsService := sessions.NewService(sessionsRepo)

	userRepo := users.NewRepository(db)
	userService := users.NewService(userRepo, sessionsService)
	userHandler := users.NewHandler(userService, sessionsService)
	userHandler.RegisterRoutes(r)

	fileRepo := files.NewRepository(db)
	fileService := files.NewService(fileRepo)
	fileHandler := files.NewHandler(fileService)
	fileHandler.RegisterRoutes(r)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", 4000),
		Handler: r,
	}

	fmt.Println("Server Running...")
	err = srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		fmt.Println(err)
	}

	fmt.Println("Server Stopped")
}

func healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	env := types.Envelope{
		"status": "available",
	}

	err := helper.WriteJSON(w, http.StatusOK, env, nil)
	if err != nil {
		fmt.Println(err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
