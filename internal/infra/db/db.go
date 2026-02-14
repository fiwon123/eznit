package db

import (
	"fmt"
	"log"
	"os"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type Config struct {
	conn *sqlx.DB
}

func New() *Config {
	return &Config{
		conn: nil,
	}
}

func (config *Config) Open() {
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	pwd := os.Getenv("DB_PWD")
	name := os.Getenv("DB_NAME")

	connStr := fmt.Sprintf("postgresql://%s:%s@localhost:%s/%s?sslmode=disable", user, pwd, port, name)

	sqlDB, err := sqlx.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	err = sqlDB.Ping()
	if err != nil {
		fmt.Println("Unable to connect to PostgreSQL!")
		log.Fatal(err)
	}
	fmt.Println("Connected to PostgreSQL successfully!")

	config.conn = sqlDB
}

func (config *Config) Close() {
	config.conn.Close()
}
