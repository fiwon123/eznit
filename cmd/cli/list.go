package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/fiwon123/eznit/internal/domain/files"
	"github.com/fiwon123/eznit/pkg/types"
)

type ListCmd struct {
	All bool `help:"list all available files"`
}

func (cmd *ListCmd) Run(g *Globals) error {
	token, err := getToken()
	if err != nil {
		return fmt.Errorf("not logged in")
	}

	if cmd.All {
		sendListRequest(g.api.baseURL, false, token, g)
	} else {
		sendListRequest(g.api.baseURL, true, token, g)
	}

	return nil
}

func sendListRequest(baseURL string, onlyMe bool, token string, g *Globals) {

	url := baseURL + "/v1/files"
	if onlyMe {
		url = baseURL + "/v1/files/me"
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		g.logger.Warn("failed to send request to " + url)
		return
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req.Header.Set("Authorization", "Bearer "+token)

	g.logger.Debug("request: ", slog.String("url", url))
	resp, err := client.Do(req)
	if err != nil {
		g.logger.Warn("request failed: %s", slog.String("error", err.Error()))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var response types.Envelope

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			g.logger.Error("failed to decode ", slog.String("error", err.Error()))
			return
		}

		g.logger.Warn("signup if not registered and/or login to generate a new token", slog.Any("result", response))
		return
	}

	var response files.ListResponse

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		g.logger.Error("failed to decode ", slog.String("error", err.Error()))
		return
	}

	for i := 0; i < response.Total; i++ {
		fmt.Println(response.Data[i])
	}

}
