package handlers

import (
	"net/http"

	"github.com/fiwon123/eznit/internal/app"
	"github.com/julienschmidt/httprouter"
)

type handlersData struct {
	app *app.AppData
}

func NewHandlers(app *app.AppData) *handlersData {
	return &handlersData{
		app: app,
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
