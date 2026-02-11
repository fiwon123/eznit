package services

import (
	"github.com/fiwon123/eznit/internal/infra/db"
)

type ServicesData struct {
	db *db.DbData
}

func NewServices(db *db.DbData) *ServicesData {
	return &ServicesData{
		db: db,
	}
}
