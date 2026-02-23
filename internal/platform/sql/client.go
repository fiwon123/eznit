package sql

import (
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func Open(driver string, dsn string) (*sqlx.DB, error) {
	db, err := sqlx.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return db, nil
}
