package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/internal/domain/users"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/types"
)

// Login Cmd keep token storaged
type LoginCmd struct {
}

func (cmd *LoginCmd) Run(g *Globals) error {
	fmt.Println("Login")

	var email string

	fmt.Print("Enter Email: ")
	fmt.Scan(&email)

	password, err := promptPassword("Enter Password: ")
	if err != nil {
		g.logger.Warn("promp password. ", slog.String("error", err.Error()))
		return nil
	}

	token, ok := sendLoginRequest(g.api.baseURL, users.LoginRequest{
		Email:    email,
		Password: password,
	}, g)
	if !ok {
		return nil
	}

	g.logger.Info("account logged in! ")

	g.logger.Debug("token to save", slog.String("token_fp", helper.Fingerprint(token)))
	err = saveToken(token)
	if err != nil {
		g.logger.Warn("failed to save token. ")
		return nil
	}

	return nil
}

func sendLoginRequest(baseURL string, request users.LoginRequest, g *Globals) (string, bool) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		g.logger.Error("failed to convert request to json. ", slog.Any("error", err))
		return "", false
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	fmt.Println()
	resp, err := client.Post(baseURL+"/v1/login", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		g.logger.Warn("failed to send request to " + baseURL + "/v1/login ")
		return "", false
	}
	defer resp.Body.Close()

	var envelope types.Envelope[sessions.DataResponse]

	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		g.logger.Error("failed to decode. ", slog.String("error", err.Error()))
		return "", false
	}

	if resp.StatusCode != http.StatusOK {
		g.logger.Warn("signup if not registered. ", slog.Any("result", envelope))

		return "", false
	}

	g.logger.Debug("response",
		slog.Int("status_code", resp.StatusCode),
		slog.String("message", envelope.Msg),
	)

	return envelope.Data.Token, true
}

// saveToken stores the session token next to whatever else the config file
// holds. It replaces the token and only the token.
func saveToken(token string) error {
	return setConfigValue("token", token)
}
