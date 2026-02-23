package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type UploadCmd struct {
	Args []string `arg:"" optional:"" help:"args for upload"`
}

func (cmd *UploadCmd) Run(g *Globals) error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("file path: ")
	path, _ := reader.ReadString('\n')
	path = strings.TrimSpace(path)

	token, err := getToken()
	if err != nil {
		return fmt.Errorf("not logged in")
	}

	err = uploadFile(g.BaseURL+"/v1/files", path, token)
	if err != nil {
		return fmt.Errorf("%s", err.Error())
	}

	return nil
}

func uploadFile(url string, filePath string, token string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return err
	}
	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
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
