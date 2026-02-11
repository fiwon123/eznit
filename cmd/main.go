package main

import (
	"flag"
	"fmt"
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
	defer dbData.Close()

	app := app.NewApp(port, dbData)

	err := api.Serve(app)
	if err != nil {
		fmt.Println(err)
	}
}

func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

}
