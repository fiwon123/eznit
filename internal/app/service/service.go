package service

import (
	"github.com/fiwon123/eznit/internal/infra/db"
)

type Config struct {
	db *db.Config
}

func New(db *db.Config) *Config {
	return &Config{
		db: db,
	}
}
