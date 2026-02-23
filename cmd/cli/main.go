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
	BaseURL string `help:"The base URL for the API" default:"http://localhost:4000"`
}

type CLI struct {
	Globals

	Login    LoginCmd    `cmd:"" aliases:"l" help:"save user credential"`
	Signup   SignupCmd   `cmd:"" aliases:"s" help:"create new user"`
	Download DownloadCmd `cmd:"" aliases:"d" help:"download a file"`
	Upload   UploadCmd   `cmd:"" aliases:"u" help:"upload a file"`
	List     ListCmd     `cmd:"" help:"list files"`
}

func main() {
	_ = godotenv.Load()

	cli := CLI{}
	ctx := kong.Parse(&cli,
		kong.Bind(&cli.Globals))
	cli.BaseURL = os.Getenv("API_URL")

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
