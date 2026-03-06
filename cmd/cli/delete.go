package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fiwon123/eznit/pkg/types"
)

type DeleteCmd struct {
}

func (cmd *DeleteCmd) Run(g *Globals) error {
	fmt.Println("delete")

	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in. ")
		return nil
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("id file: ")
	id, _ := reader.ReadString('\n')
	id = strings.TrimSpace(id)

	fmt.Println()
	if id == "" {
		g.logger.Warn("id is empty. ")
		return nil
	}

	sendRequestDelete(g.api.baseURL, id, token, g)

	return nil
}

func sendRequestDelete(baseURL string, id string, token string, g *Globals) {

	url := baseURL + "/v1/files/" + id
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		g.logger.Warn("failed to create new request. ", slog.String("error", err.Error()))
		return
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		g.logger.Warn("failed to send request. ", slog.String("error", err.Error()))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var response types.Envelope[any]

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			g.logger.Error("failed to decode. ", slog.String("error", err.Error()))
			return
		}

		g.logger.Warn("delete failed", slog.Any("result", response))

		return
	}

	g.logger.Info("file deleted!")
}
