package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fiwon123/eznit/pkg/types"
)

type UploadCmd struct {
	Update string `help:"update a file using id"`
}

func (cmd *UploadCmd) Run(g *Globals) error {
	fmt.Println("upload")

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("file path: ")
	path, _ := reader.ReadString('\n')
	path = strings.TrimSpace(path)

	fmt.Println()
	token, err := getToken()
	if err != nil {
		g.logger.Info("not logged in")
		return nil
	}

	if cmd.Update != "" {
		updateFile(g.api.baseURL, path, cmd.Update, token, g)
	} else {
		uploadFile(g.api.baseURL, path, token, g)
	}

	return nil
}

func updateFile(baseURL string, filePath string, id string, token string, g *Globals) {
	file, err := os.Open(filePath)
	if err != nil {
		g.logger.Warn("failed to open file path. ", slog.String("error", err.Error()))
		return
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		g.logger.Warn("failed to create form file. ", slog.String("error", err.Error()))
		return
	}

	_, err = io.Copy(part, file)
	if err != nil {
		g.logger.Warn("failed to copy file content to request. ", slog.String("error", err.Error()))
		return
	}
	writer.Close()

	url := baseURL + "/v1/files/" + id
	req, err := http.NewRequest("PUT", url, body)
	if err != nil {
		g.logger.Warn("failed to wraps new request. ", slog.String("error", err.Error()))
		return
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
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

	g.logger.Info("updated file. ")

}

func uploadFile(baseURL string, filePath string, token string, g *Globals) {
	file, err := os.Open(filePath)
	if err != nil {
		g.logger.Warn("failed to open file path. ", slog.String("error", err.Error()))
		return
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		g.logger.Warn("failed to create form file. ", slog.String("error", err.Error()))
		return
	}

	_, err = io.Copy(part, file)
	if err != nil {
		g.logger.Warn("failed to copy file content to request. ", slog.String("error", err.Error()))
		return
	}
	writer.Close()

	url := baseURL + "/v1/files"
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		g.logger.Warn("failed to wraps new request. ", slog.String("error", err.Error()))
		return
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
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
		var response types.Envelope

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			g.logger.Error("failed to decode. ", slog.String("error", err.Error()))
			return
		}

		g.logger.Warn("signup if not registered. ", slog.Any("result", response))

		return
	}

	g.logger.Info("uploaded file. ")
}
