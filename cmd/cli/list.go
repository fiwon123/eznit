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

// Login Cmd show all files uploaded any user
// if used me parameter only filed uploaded by user will be listed
type ListCmd struct {
	Me bool `help:"list all available files"`
}

func (cmd *ListCmd) Run(g *Globals) error {
	fmt.Println("list")

	fmt.Println()

	if cmd.Me {
		token, err := getToken()
		if err != nil {
			g.logger.Warn("not logged in! ")
			return nil
		}

		sendListRequest(g.api.baseURL, true, token, g)

	} else {
		sendListRequest(g.api.baseURL, false, "", g)
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

	g.logger.Debug("request", slog.String("url", url))
	resp, err := client.Do(req)
	if err != nil {
		g.logger.Warn("request failed. ", slog.String("error", err.Error()))
		return
	}
	defer resp.Body.Close()

	var envelope types.Envelope[files.ListResponse]
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		g.logger.Error("failed to decode. ", slog.String("error", err.Error()))
		return
	}

	if resp.StatusCode != http.StatusOK {

		g.logger.Warn("signup if not registered and/or login to generate a new token. ", slog.Any("result", envelope))
		return
	}

	data := envelope.Data
	for i := 0; i < data.Total; i++ {
		fmt.Println(data.Data[i])
	}

}
