package main

import (
	"flag"
	"log"

	"github.com/fiwon123/eznit/internal/api"
	"github.com/fiwon123/eznit/internal/app"
	"github.com/fiwon123/eznit/internal/infra/db"
	"github.com/joho/godotenv"
)

func main() {

	loadEnv()

	var port int
	flag.IntVar(&port, "port", 4000, "API server port")

	dbData := db.NewDB()
	dbData.Open()

	app := app.NewApp(port, dbData)

	api.Serve(app)
}

func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

}
