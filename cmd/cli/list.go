package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fiwon123/eznit/internal/domain/files"
)

type ListCmd struct {
	All  bool     `help:"list all available files"`
	Args []string `arg:"" optional:"" help:"args for list"`
}

func (cmd *ListCmd) Run(g *Globals) error {
	token, err := getToken()
	if err != nil {
		return fmt.Errorf("not logged in")
	}

	if cmd.All {
		sendListRequest(g.BaseURL, false, token)
	} else {
		sendListRequest(g.BaseURL, true, token)
	}

	return nil
}

func sendListRequest(baseURL string, onlyMe bool, token string) error {

	url := baseURL + "/v1/files"
	if onlyMe {
		url = baseURL + "/v1/files/me"
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req.Header.Set("Authorization", "Bearer "+token)

	fmt.Println("request: ", url)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned error: %s", resp.Status)
	}

	var response files.ListResponse

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("error decode")
	}

	for i := 0; i < response.Total; i++ {
		fmt.Println(response.Data[i])
	}

	return nil
}
