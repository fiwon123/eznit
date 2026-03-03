package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fiwon123/eznit/internal/domain/users"
	"github.com/fiwon123/eznit/pkg/types"
)

type LoginCmd struct {
}

func (cmd *LoginCmd) Run(g *Globals) error {
	fmt.Println("Login")

	var email string

	fmt.Print("Enter Email: ")
	fmt.Scan(&email)

	password, err := promptPassword("Enter Password: ")
	if err != nil {
		g.logger.Warn("promp password", slog.String("error", err.Error()))
		return nil
	}

	response, ok := sendLoginRequest(g.api.baseURL, users.LoginRequest{
		Email:    email,
		Password: password,
	}, g)
	if !ok {
		return nil
	}

	g.logger.Info("account logged in!")

	err = saveToken(response.Token)
	if err != nil {
		g.logger.Warn("failed to save token")
		return nil
	}

	return nil
}

func sendLoginRequest(baseURL string, request users.LoginRequest, g *Globals) (users.LoginResponse, bool) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		g.logger.Error("failed to convert request to json ", slog.Any("error", err))
		return users.LoginResponse{}, false
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	fmt.Println()
	resp, err := client.Post(baseURL+"/v1/login", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		g.logger.Warn("failed to send request to " + baseURL + "/v1/login")
		return users.LoginResponse{}, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var response types.Envelope

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			g.logger.Error("failed to decode ", slog.String("error", err.Error()))
			return users.LoginResponse{}, false
		}

		g.logger.Warn("signup if not registered", slog.Any("result", response))

		return users.LoginResponse{}, false
	}

	var tokenResp users.LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		g.logger.Error("failed to decode", slog.String("error", err.Error()))
		return users.LoginResponse{}, false
	}

	return tokenResp, true
}

func saveToken(token string) error {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".eznit")
	configPath := filepath.Join(configDir, "config.json")

	os.MkdirAll(configDir, 0700)

	config := map[string]string{"token": token}
	data, _ := json.MarshalIndent(config, "", "  ")

	return os.WriteFile(configPath, data, 0600)
}
