package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/joho/godotenv"
)

type API struct {
	baseURL string
	host    string
	port    string
}

func newAPI(host string, port string) *API {
	return &API{
		baseURL: host + ":" + port,
		host:    host,
		port:    port,
	}
}

type Globals struct {
	api       *API
	downloads string
	logger    *logger.Config
}

func newGlobals(api *API, downloads string, logger *logger.Config) *Globals {
	return &Globals{
		api:       api,
		downloads: downloads,
		logger:    logger,
	}
}

type CLI struct {
	Debug   bool             `help:"enable debug level"`
	Version kong.VersionFlag `short:"v" help:"show version."`

	Login    LoginCmd    `cmd:"" aliases:"l" help:"save user credential"`
	Signup   SignupCmd   `cmd:"" aliases:"s" help:"create new user"`
	Download DownloadCmd `cmd:"" aliases:"d" help:"download a file"`
	Upload   UploadCmd   `cmd:"" aliases:"u" help:"upload a file"`
	List     ListCmd     `cmd:"" help:"list files"`
	Delete   DeleteCmd   `cmd:"" help:"delete file"`
	Logout   LogoutCmd   `cmd:"" help:"logout user and remove credential"`
}

var globals Globals
var Version = "dev"

func main() {

	// .env.local overwrite .env for development
	_ = godotenv.Load(".env.local", ".env")

	if len(os.Args) < 2 {
		os.Args = append(os.Args, "--help")
	}

	cli := CLI{}
	ctx := kong.Parse(&cli, kong.Vars{"version": Version})

	logsFolder, _ := os.LookupEnv("CLI_LOGS")
	if logsFolder == "" {
		logsFolder = "./logs/"
	}
	l, err := logger.NewConsole(logsFolder, cli.Debug, false)
	if err != nil {
		log.Fatal(err)
	}
	defer l.Sync()

	host, _ := os.LookupEnv("CLI_API_HOST")
	if host == "" {
		host = "http://localhost"
	}

	port, _ := os.LookupEnv("CLI_API_PORT")
	if port == "" {
		port = "4000"
	}

	downloads, _ := os.LookupEnv("CLI_DOWNLOADS")
	if downloads == "" {
		downloads = "./downloads/"
	}

	api := newAPI(host, port)
	globals = *newGlobals(api, downloads, l)

	ctx.Run(&globals)
}

func getToken() (string, error) {
	home, _ := os.UserHomeDir()
	tokenPath := filepath.Join(home, ".eznit", "config.json")
	tokenRaw, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", err
	}

	var tokenMap map[string]string
	json.Unmarshal(tokenRaw, &tokenMap)

	token, ok := tokenMap["token"]
	if !ok {
		return "", fmt.Errorf("not logged in: %v", err)
	}

	return token, nil
}
