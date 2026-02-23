package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fiwon123/eznit/internal/domain/files"
)

type ListCmd struct {
	Args []string `arg:"" optional:"" help:"args for list"`
}

func (cmd *ListCmd) Run(g *Globals) error {
	token, err := getToken()
	if err != nil {
		return fmt.Errorf("not logged in")
	}

	sendListRequest(g.BaseURL, token)

	return nil
}

func sendListRequest(baseURL string, token string) error {

	req, err := http.NewRequest("GET", baseURL+"/v1/files/me", nil)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req.Header.Set("Authorization", "Bearer "+token)

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
