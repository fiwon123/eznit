package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/fiwon123/eznit/internal/api/handlers"
	"github.com/fiwon123/eznit/internal/app"
)

func Serve(app *app.AppData) error {
	h := handlers.NewHandlers(app.Services())

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", app.Cfg().Port()),
		Handler: h.AllHandlers(),
	}

	fmt.Println("Server Running...")
	err := srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	fmt.Println("Server Stopped")
	return nil
}
