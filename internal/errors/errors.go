package errors

import (
	"fmt"
	"net/http"

	"github.com/fiwon123/eznit/internal/data/app"
)

func ServerErrorResponse(w http.ResponseWriter, r *http.Request, err error, app *app.Data) {
	// app.logError(r, err)
	message := "the server encountered a problem and could not process your request"
	fmt.Println(message)
	// app.errorResponse(w, r, http.StatusInternalServerError, message)
}
