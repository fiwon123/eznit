package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"
)

type Globals struct {
	BaseURL   string `help:"The base URL for the API" default:"http://localhost:4000"`
	Downloads string `help:"The base URL for the API" default:"./downloads"`
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

	val, ok := os.LookupEnv("API_URL")
	if ok {
		cli.BaseURL = val
	}

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
