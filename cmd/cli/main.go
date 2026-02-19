package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
)

type CLI struct {
	Login  LoginCmd  `cmd:"" aliases:"l" help:"save user credential"`
	Signup SignupCmd `cmd:"" aliases:"s" help:"create new user"`
}

func main() {
	cli := CLI{}
	ctx := kong.Parse(&cli)
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}

func getToken() (map[string]string, error) {
	home, _ := os.UserHomeDir()
	tokenPath := filepath.Join(home, ".eznit", "config.json")
	tokenRaw, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("not logged in: %v", err)
	}

	var token map[string]string
	json.Unmarshal(tokenRaw, &token)

	return token, nil
}
