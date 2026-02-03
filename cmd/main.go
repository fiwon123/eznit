package main

import (
	"flag"
	"log"

	"github.com/fiwon123/eznit/internal/data/app"
	"github.com/fiwon123/eznit/internal/db"
	"github.com/fiwon123/eznit/internal/server"
	"github.com/joho/godotenv"
)

func main() {

	loadEnv()

	var port int
	flag.IntVar(&port, "port", 4000, "API server port")

	app := app.New(port)

	db.Open()

	server.Serve(app)
}

func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

}
