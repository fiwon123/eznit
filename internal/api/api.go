package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/fiwon123/eznit/internal/api/handler"
	"github.com/fiwon123/eznit/internal/app"
)

func Serve(app *app.Config) error {
	h := handler.New(app.Service())

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", app.Port()),
		Handler: h.Routes(),
	}

	fmt.Println("Server Running...")
	err := srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	fmt.Println("Server Stopped")
	return nil
}
