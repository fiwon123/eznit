package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/joho/godotenv"
)

type API struct {
	BaseURL string `help:"The complete url with port" default:"http://localhost:4000"`
	Host    string `help:"The host URL" default:"http://localhost"`
	Port    string `help:"The API port" default:"4000"`
}

type Globals struct {
	API       API
	Downloads string `help:"default downloads folder" default:"./downloads"`
	Logger    *logger.Config
}

type CLI struct {
	Globals

	Login    LoginCmd    `cmd:"" aliases:"l" help:"save user credential"`
	Signup   SignupCmd   `cmd:"" aliases:"s" help:"create new user"`
	Download DownloadCmd `cmd:"" aliases:"d" help:"download a file"`
	Upload   UploadCmd   `cmd:"" aliases:"u" help:"upload a file"`
	List     ListCmd     `cmd:"" help:"list files"`
	Delete   DeleteCmd   `cmd:"" help:"delete file"`
}

func main() {
	_ = godotenv.Load()

	cli := CLI{}
	ctx := kong.Parse(&cli,
		kong.Bind(&cli.Globals))

	cli.Logger = logger.New(false)
	cli.Logger.Info(
		"Logger initialized!",
		slog.Int("process_id", os.Getpid()),
	)
	defer cli.Logger.Sync()

	val, ok := os.LookupEnv("API_HOST")
	if ok {
		cli.API.BaseURL = val
	}

	val, ok = os.LookupEnv("API_PORT")
	if ok {
		cli.API.Port = val
	}

	cli.API.BaseURL = fmt.Sprintf("%s:%s", cli.API.Host, cli.API.Port)

	val, ok = os.LookupEnv("CLI_DOWNLOADS")
	if ok {
		cli.Downloads = val
	}

	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}

func getToken() (string, error) {
	home, _ := os.UserHomeDir()
	tokenPath := filepath.Join(home, ".eznit", "config.json")
	tokenRaw, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", fmt.Errorf("not logged in: %v", err)
	}

	var tokenMap map[string]string
	json.Unmarshal(tokenRaw, &tokenMap)

	token, ok := tokenMap["token"]
	if !ok {
		return "", fmt.Errorf("not logged in: %v", err)
	}

	return token, nil
}
