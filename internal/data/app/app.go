package app

import (
	"github.com/fiwon123/eznit/internal/data/cfg"
)

type Data struct {
	cfg *cfg.Data
}

func New(port int) *Data {
	return &Data{
		cfg: cfg.New(port),
	}
}

func (app *Data) Cfg() *cfg.Data {
	return app.cfg
}
