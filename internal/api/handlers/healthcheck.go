package handlers

import (
	"net/http"

	"github.com/fiwon123/eznit/pkg/errors"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/types"
)

func (handlers *handlersData) healthcheckHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env := types.Envelope{
			"status": "available",
		}

		err := helper.WriteJSON(w, http.StatusOK, env, nil)
		if err != nil {
			errors.ServerErrorResponse(w, r, err)
		}
	}
}
