package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const ()

func getToken() ([]byte, error) {
	home, _ := os.UserHomeDir()
	tokenPath := filepath.Join(home, ".eznit", "token")
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("not logged in: %v", err)
	}

	return token, nil
}
