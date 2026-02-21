package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
)

type CLI struct {
	Login    LoginCmd    `cmd:"" aliases:"l" help:"save user credential"`
	Signup   SignupCmd   `cmd:"" aliases:"s" help:"create new user"`
	Download DownloadCmd `cmd:"" aliases:"d" help:"download a file"`
	Upload   UploadCmd   `cmd:"" aliases:"u" help:"upload a file"`
}

func main() {
	cli := CLI{}
	ctx := kong.Parse(&cli)
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
