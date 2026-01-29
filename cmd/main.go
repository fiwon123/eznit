package main

import (
	"flag"

	"github.com/fiwon123/eznit/internal/data/app"
	"github.com/fiwon123/eznit/internal/server"
)

func main() {

	var port int
	flag.IntVar(&port, "port", 4000, "API server port")

	app := app.New(port)

	server.Serve(app)
}
