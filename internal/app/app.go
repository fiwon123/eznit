package app

import (
	"github.com/fiwon123/eznit/internal/app/services"
	"github.com/fiwon123/eznit/internal/cfg"
	"github.com/fiwon123/eznit/internal/infra/db"
)

type AppData struct {
	cfg      *cfg.AppCfg
	db       *db.DbData
	services *services.ServicesData
}

func NewApp(port int, db *db.DbData) *AppData {
	return &AppData{
		cfg:      cfg.NewAppCfg(port),
		db:       db,
		services: services.NewServices(db),
	}
}

func (app *AppData) Cfg() *cfg.AppCfg {
	return app.cfg
}

func (app *AppData) DB() *db.DbData {
	return app.db
}

func (app *AppData) Services() *services.ServicesData {
	return app.services
}
