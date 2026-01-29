package server

import (
	"net/http"

	"github.com/fiwon123/eznit/internal/data/app"
	"github.com/fiwon123/eznit/internal/server/routes"
	"github.com/julienschmidt/httprouter"
)

func allRoutes(app *app.Data) http.Handler {
	router := httprouter.New()
	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", routes.HealthcheckHandler(app))

	return router
}
