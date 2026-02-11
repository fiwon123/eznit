package db

import (
	"fmt"
	"log"
	"os"

	"database/sql"

	_ "github.com/lib/pq"
)

type DbData struct {
	sqlDB *sql.DB
}

func NewDB() *DbData {
	return &DbData{
		sqlDB: nil,
	}
}

func (db *DbData) Open() {
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	pwd := os.Getenv("DB_PWD")
	name := os.Getenv("DB_NAME")

	connStr := fmt.Sprintf("postgresql://%s:%s@localhost:%s/%s?sslmode=disable", user, pwd, port, name)

	sqlDB, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	err = sqlDB.Ping()
	if err != nil {
		fmt.Println("Unable to connect to PostgreSQL!")
		log.Fatal(err)
	}
	fmt.Println("Connected to PostgreSQL successfully!")

	db.sqlDB = sqlDB
}

func (db *DbData) Close() {
	db.sqlDB.Close()
}
