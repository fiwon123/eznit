package handlers

import (
	"net/http"

	"github.com/fiwon123/eznit/internal/app/services"
	"github.com/go-chi/chi/v5"
)

type handlersData struct {
	services *services.ServicesData
}

func NewHandlers(services *services.ServicesData) *handlersData {
	return &handlersData{
		services: services,
	}
}

func (handlers *handlersData) AllHandlers() http.Handler {
	r := chi.NewRouter()
	r.Get("/v1/healthcheck", handlers.healthcheckHandler)
	r.Get("/v1/users", handlers.getUsersHandler)
	r.Get("/v1/users/{id}", handlers.getUserHandler)
	r.Post("/v1/users", handlers.createUserHandler)
	r.Delete("/v1/users/{id}", handlers.deleteUserHandler)
	r.Put("/v1/users/{id}", handlers.updateUserHandler)

	return r
}
