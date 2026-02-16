package sql

import (
	"fmt"

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
		fmt.Println("Unable to connect to PostgreSQL!")
		return nil, err
	}
	fmt.Println("Connected to PostgreSQL successfully!")

	return db, nil
}
