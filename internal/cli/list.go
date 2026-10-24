package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/fiwon123/eznit/internal/domain/files"
	"github.com/fiwon123/eznit/pkg/types"
)

// ListCmd lists the caller's own files: a file belongs to the user who
// uploaded it, so there is no instance-wide list to ask for.
type ListCmd struct {
	// Me is kept for the clients that name their own files explicitly. It
	// selects the same list as the default, now that the wide route is gone.
	Me bool `help:"list only your own files (the default)"`
}

func (cmd *ListCmd) Run(g *Globals) error {
	fmt.Println("list")

	fmt.Println()

	// Both routes are behind the session guard: the list answers the
	// caller's own files, so it is not public.
	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in! ")
		return nil
	}

	sendListRequest(g.api.baseURL, cmd.Me, token, g)

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
