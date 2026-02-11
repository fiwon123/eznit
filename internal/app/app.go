package app

import (
	"github.com/fiwon123/eznit/internal/app/service"
	"github.com/fiwon123/eznit/internal/infra/db"
)

type Config struct {
	port    int
	db      *db.Config
	service *service.Config
}

func New(port int, db *db.Config) *Config {
	return &Config{
		port:    port,
		db:      db,
		service: service.New(db),
	}
}

func (app *Config) Port() int {
	return app.port
}

func (app *Config) DB() *db.Config {
	return app.db
}

func (app *Config) Service() *service.Config {
	return app.service
}
