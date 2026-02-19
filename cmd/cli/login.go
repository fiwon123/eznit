package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fiwon123/eznit/internal/domain/users"
)

type LoginCmd struct {
	Args []string `arg:"" optional:"" help:"args for login"`
}

func (cmd *LoginCmd) Run() error {
	var email string

	fmt.Print("Enter Email: ")
	fmt.Scan(&email)

	password, err := promptPassword("Enter Password: ")
	if err != nil {
		return err
	}

	err = sendLoginRequest(users.LoginRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		return fmt.Errorf("internal server error")
	}

	fmt.Println("Account Logged in!")
	return nil
}

func sendLoginRequest(request users.LoginRequest) error {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Post("http://localhost:4000/v1/login", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned error: %s", resp.Status)
	}

	return nil
}

func (cmd *LoginCmd) SaveToken(token string) error {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".eznit")

	os.MkdirAll(configDir, 0700)

	return os.WriteFile(filepath.Join(configDir, "token"), []byte(token), 0600)
}
