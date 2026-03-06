package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/pkg/types"
)

type LogoutCmd struct {
}

func (cmd *LogoutCmd) Run(g *Globals) error {
	fmt.Println("logout")

	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in! ")
		return nil
	}

	ok := sendLogoutRequest(g.api.baseURL, token, g)
	if !ok {
		return nil
	}

	g.logger.Info("logout ok")
	deleteToken()

	return nil
}

func sendLogoutRequest(baseURL string, token string, g *Globals) bool {

	g.logger.Debug("token", slog.String("token", token))

	req, err := http.NewRequest("GET", baseURL+"/v1/logout", nil)
	if err != nil {
		g.logger.Warn("failed to send request to " + baseURL + "/v1/logout")
		return false
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	req.Header.Set("Authorization", "Bearer "+token)

	fmt.Println()
	resp, err := client.Do(req)
	if err != nil {
		g.logger.Warn("failed to send request to " + baseURL + "/v1/logout")
		return false
	}
	defer resp.Body.Close()

	var envelope types.Envelope[sessions.DataResponse]

	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		g.logger.Error("failed to decode. ", slog.String("error", err.Error()))
		return false
	}

	if resp.StatusCode != http.StatusOK {
		g.logger.Warn("status error", slog.Any("result", envelope))
		return false
	}

	return true
}

func deleteToken() error {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".eznit")
	configPath := filepath.Join(configDir, "config.json")

	os.MkdirAll(configDir, 0700)

	config := map[string]string{}
	data, _ := json.MarshalIndent(config, "", "  ")

	return os.WriteFile(configPath, data, 0600)
}
