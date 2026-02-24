package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type DeleteCmd struct {
	Args []string `arg:"" optional:"" help:"args for delete"`
}

func (cmd *DeleteCmd) Run(g *Globals) error {

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("id file: ")
	id, _ := reader.ReadString('\n')
	id = strings.TrimSpace(id)

	if id == "" {
		return fmt.Errorf("id is empty")
	}

	token, err := getToken()
	if err != nil {
		return fmt.Errorf("not logged in")
	}

	err = sendRequestDelete(g.api.baseURL, id, token)

	return err
}

func sendRequestDelete(baseURL string, id string, token string) error {

	url := baseURL + "/v1/files/" + id
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
