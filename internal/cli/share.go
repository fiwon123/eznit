package cli

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

// Share Cmd is used to share/unshare a file
type ShareCmd struct {
	Off bool `name:"off" help:"unshare the file"`
}

// Unshare Cmd is used to unshare a file
type UnshareCmd struct {
}

func (cmd *ShareCmd) Run(g *Globals) error {
	fmt.Println("share")

	fmt.Println()
	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in. ")
		return nil
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("id file: ")
	id, _ := reader.ReadString('\n')
	id = strings.TrimSpace(id)

	if id == "" {
		g.logger.Warn("id is empty. ")
		return nil
	}

	if !cmd.Off {
		fmt.Print("Make this file public? Anyone on this instance can download it. [y/N]: ")
		ans, _ := reader.ReadString('\n')
		ans = strings.TrimSpace(strings.ToLower(ans))
		if ans != "y" && ans != "yes" {
			fmt.Println("cancelled")
			return nil
		}
	}

	sendRequestShare(g.api.baseURL, id, token, !cmd.Off, g)

	return nil
}

func (cmd *UnshareCmd) Run(g *Globals) error {
	fmt.Println("unshare")

	fmt.Println()
	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in. ")
		return nil
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("id file: ")
	id, _ := reader.ReadString('\n')
	id = strings.TrimSpace(id)

	if id == "" {
		g.logger.Warn("id is empty. ")
		return nil
	}

	sendRequestShare(g.api.baseURL, id, token, false, g)

	return nil
}

func sendRequestShare(baseURL string, id string, token string, isPublic bool, g *Globals) {
	url := baseURL + "/v1/files/" + id + "/share"
	body, _ := json.Marshal(map[string]bool{"is_public": isPublic})
	req, err := http.NewRequest("POST", url, strings.NewReader(string(body)))
	if err != nil {
		g.logger.Warn("failed to create new request. ", slog.String("error", err.Error()))
		return
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

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

		g.logger.Warn("share failed", slog.Any("result", response))

		return
	}

	var response types.Envelope[any]
	if err := json.NewDecoder(resp.Body).Decode(&response); err == nil {
		if response.Msg != "" {
			g.logger.Info(response.Msg)
			return
		}
	}

	if isPublic {
		g.logger.Info("file shared!")
		return
	}
	g.logger.Info("file unshared!")
}
