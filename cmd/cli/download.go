package main

import (
	"bufio"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fiwon123/eznit/pkg/helper"
)

type DownloadCmd struct {
	Args []string `arg:"" optional:"" help:"args for download"`
}

func (cmd *DownloadCmd) Run(g *Globals) error {

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

	if dest == "" {
		dest = g.Downloads
		fmt.Println("destination folder is empty, default download folder path: ", dest)
	}

	err := helper.CreatePathIfNotExists(dest)
	if err != nil {
		return fmt.Errorf("can't create destination folder path")
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:4000/v1/files/%s/content", id), nil)
	if err != nil {
		return err
	}

	token, err := getToken()
	if err != nil {
		return fmt.Errorf("not logged in")
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

	split := strings.Split(filename, ".")
	name := split[0]
	ext := ""
	if len(split) > 1 {
		ext = split[1]
	}

	fullpath := filepath.Join(dest, filename)
	fmt.Println(fullpath)

	counter := 1
	exists, _ := helper.PathExists(fullpath)
	fmt.Println("exists: ", exists)
	for exists {
		fullpath = filepath.Join(dest, fmt.Sprintf("%s_%d.%s", name, counter, ext))

		fmt.Println("for ", fullpath)
		exists, _ = helper.PathExists(fullpath)
		counter += 1
	}

	fmt.Println(fullpath)
	out, err := os.Create(fullpath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
