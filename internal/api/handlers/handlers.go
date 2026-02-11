package handlers

import (
	"net/http"

	"github.com/fiwon123/eznit/internal/app/services"
	"github.com/julienschmidt/httprouter"
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
	router := httprouter.New()
	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", handlers.healthcheckHandler())
	router.HandlerFunc(http.MethodGet, "/v1/users", handlers.getUsersHandler())
	router.HandlerFunc(http.MethodGet, "/v1/users/{id}", handlers.getUserHandler())
	router.HandlerFunc(http.MethodPost, "/v1/users", handlers.createUserHandler())
	router.HandlerFunc(http.MethodDelete, "/v1/users/{id}", handlers.deleteUserHandler())
	router.HandlerFunc(http.MethodPut, "/v1/users/{id}", handlers.updateUserHandler())

	return router
}
