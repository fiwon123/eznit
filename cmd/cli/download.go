package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

type DownloadCmd struct {
	Args []string `arg:"" optional:"" help:"args for download"`
}

func (cmd *DownloadCmd) Run() error {

	var id string

	fmt.Print("id file: ")
	fmt.Scan(&id)

	var dest string
	fmt.Print("\ndestination folder path: ")
	fmt.Scan(dest)

	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:4000/v1/files/%s/content", id), nil)
	if err != nil {
		return err
	}

	token, err := getToken()
	if err != nil {
		return fmt.Errorf("not logged in")
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	var filename string
	contentDisp := resp.Header.Get("Content-Disposition")
	if contentDisp != "" {
		_, params, err := mime.ParseMediaType(contentDisp)
		if err == nil {
			filename = params["filename"]
			fmt.Println("Server suggests filename:", filename)
		}
	}

	out, err := os.Create(filepath.Join(dest, filename))
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
