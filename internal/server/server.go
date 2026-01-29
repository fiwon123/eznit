package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/fiwon123/eznit/internal/data/app"
)

func Serve(app *app.Data) error {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", app.Cfg().Port()),
		Handler: allRoutes(app),
	}

	fmt.Println("Starting Server...")
	err := srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	fmt.Println("Server Stopped")
	return nil
}
