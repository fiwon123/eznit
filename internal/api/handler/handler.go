package handler

import (
	"net/http"

	"github.com/fiwon123/eznit/internal/app/service"
	"github.com/go-chi/chi/v5"
)

type Config struct {
	service *service.Config
}

func New(service *service.Config) *Config {
	return &Config{
		service: service,
	}
}

func (config *Config) AllHandlers() http.Handler {
	r := chi.NewRouter()
	r.Get("/v1/healthcheck", config.healthcheckHandler)
	r.Get("/v1/users", config.getUsersHandler)
	r.Get("/v1/users/{id}", config.getUserHandler)
	r.Post("/v1/users", config.createUserHandler)
	r.Delete("/v1/users/{id}", config.deleteUserHandler)
	r.Put("/v1/users/{id}", config.updateUserHandler)

	return r
}
