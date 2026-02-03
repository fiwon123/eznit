package db

import (
	"fmt"
	"log"
	"os"

	"database/sql"

	_ "github.com/lib/pq"
)

type DB struct {
}

func Open() {
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	pwd := os.Getenv("DB_PWD")
	name := os.Getenv("DB_NAME")

	// Define the connection string with PostgreSQL credentials
	connStr := fmt.Sprintf("postgresql://%s:%s@localhost:%s/%s?sslmode=disable", user, pwd, port, name)

	// Open a database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close() // Ensure connection closes after function ends

	// Ping to confirm connection
	err = db.Ping()
	if err != nil {
		fmt.Println("Unable to connect to PostgreSQL!")
		log.Fatal(err)
	}
	fmt.Println("Connected to PostgreSQL successfully!")
}
