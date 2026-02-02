package db

import (
	"fmt"
	"log"

	"database/sql"

	_ "github.com/lib/pq"
)

type DB struct {
}

func open() {
	// Define the connection string with PostgreSQL credentials
	connStr := "user=postgres password=postgres dbname=eznit sslmode=disable"

	// Open a database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close() // Ensure connection closes after function ends

	// Ping to confirm connection
	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connected to PostgreSQL successfully!")
}
