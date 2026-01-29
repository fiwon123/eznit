package routes

import (
	"net/http"

	"github.com/fiwon123/eznit/internal/data/app"
	"github.com/fiwon123/eznit/internal/data/types"
	"github.com/fiwon123/eznit/internal/errors"
	"github.com/fiwon123/eznit/internal/helper"
)

func HealthcheckHandler(app *app.Data) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env := types.Envelope{
			"status": "available",
		}

		err := helper.WriteJSON(w, http.StatusOK, env, nil)
		if err != nil {
			errors.ServerErrorResponse(w, r, err, app)
		}
	}
}
