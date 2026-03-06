package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fiwon123/eznit/pkg/helper"
)

type DownloadCmd struct {
}

func (cmd *DownloadCmd) Run(g *Globals) error {
	fmt.Println("download")

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("id file: ")
	id, _ := reader.ReadString('\n')
	id = strings.TrimSpace(id)

	if id == "" {
		return fmt.Errorf("id is empty")
	}

	fmt.Print("\ndestination folder path: ")
	dest, _ := reader.ReadString('\n')
	dest = strings.TrimSpace(dest)

	fmt.Println()
	if dest == "" {
		dest = g.downloads
		g.logger.Warn("destination folder is empty, default download folder path will be used. ", slog.String("default", dest))
	}

	err := helper.CreatePathIfNotExists(dest)
	if err != nil {
		g.logger.Warn("failed to create destination folder path. ", slog.String("error", err.Error()))
		return nil
	}

	url := g.api.baseURL + "/v1/files/" + id + "/content"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		g.logger.Warn("failed to create new request. ", slog.String("error", err.Error()))
		return nil
	}

	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in ", slog.String("error", err.Error()))
		return nil
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

	if resp.StatusCode != http.StatusOK {
		g.logger.Warn("bad status ", slog.String("status", resp.Status))
		return nil
	}

	var filename string
	contentDisp := resp.Header.Get("Content-Disposition")
	if contentDisp != "" {
		_, params, err := mime.ParseMediaType(contentDisp)
		if err == nil {
			filename = params["filename"]
			g.logger.Info("Server suggests filename", slog.String("filename", filename))
		}
	}

	split := strings.Split(filename, ".")
	name := split[0]
	ext := ""
	if len(split) > 1 {
		ext = split[1]
	}

	g.logger.Debug("adding to filepath", slog.String("filepath", filename))
	fullpath := filepath.Join(dest, filename)
	counter := 1
	exists, _ := helper.PathExists(fullpath)
	g.logger.Debug("exists value. ", slog.Bool("exists", exists))
	for exists {
		if ext != "" {
			fullpath = filepath.Join(dest, fmt.Sprintf("%s_%d.%s", name, counter, ext))
		} else {
			fullpath = filepath.Join(dest, fmt.Sprintf("%s_%d", name, counter))
		}

		g.logger.Debug("for ", slog.String("path", fullpath))
		exists, _ = helper.PathExists(fullpath)
		counter += 1
	}

	g.logger.Info("fullpath", slog.String("fullpath", fullpath))
	out, err := os.Create(fullpath)
	if err != nil {
		g.logger.Error("failed to create path ", slog.String("error", err.Error()))
		return nil
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		g.logger.Error("failed to copy file content ", slog.String("error", err.Error()))
		return nil
	}

	return nil
}
