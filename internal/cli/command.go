package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fiwon123/eznit/internal/domain/commands"
	"github.com/fiwon123/eznit/pkg/types"
	"github.com/google/uuid"
)

// The limits of a command, the same ones the API enforces. A file the server
// would refuse is refused here first, with the file named.
const (
	maxCommandTextBytes   = 256 * 1024
	maxCommandBinaryBytes = 10 * 1024 * 1024
)

// CommandCmd works with the command library: the scripts a user keeps once
// and pulls into templates.
type CommandCmd struct {
	Add    CommandAddCmd    `cmd:"" help:"add a file to your command library"`
	List   CommandListCmd   `cmd:"" help:"list your commands"`
	Show   CommandShowCmd   `cmd:"" help:"show one command"`
	Remove CommandRemoveCmd `cmd:"" help:"remove a command from your library"`
}

// CommandAddCmd adds a file to the library.
type CommandAddCmd struct {
	File string `arg:"" help:"the file to keep as a command"`
	Name string `help:"the name of the command (default: the file name)" optional:""`
	// A command is a thing you run, so it is executable unless the user says
	// otherwise.
	NotExecutable bool `help:"store it without the executable bit"`
	Replace       bool `help:"replace the command if it is already in the library"`
}

// Run adds the file to the library.
func (cmd *CommandAddCmd) Run(g *Globals) error {
	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in. ")
		return nil
	}

	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		name = filepath.Base(cmd.File)
	}

	info, err := os.Stat(cmd.File)
	if err != nil {
		g.logger.Warn("failed to read the file. ", slog.String("error", err.Error()))
		return nil
	}

	if info.IsDir() {
		g.logger.Warn("that is a folder, not a file. ", slog.String("path", cmd.File))
		return nil
	}

	// Nothing is read into memory before it is known it can fit: the binary
	// cap is the largest thing a command can be.
	if info.Size() > maxCommandBinaryBytes {
		g.logger.Warn("the file is too big to be a command. ", slog.String("path", cmd.File))
		return nil
	}

	content, err := os.ReadFile(cmd.File)
	if err != nil {
		g.logger.Warn("failed to read the file. ", slog.String("error", err.Error()))
		return nil
	}

	executable := !cmd.NotExecutable

	request := commands.CommandRequest{
		Name:         name,
		Kind:         commands.CommandKindText,
		Content:      string(content),
		IsExecutable: &executable,
	}

	if isBinary(content) {
		request.Kind = commands.CommandKindBinary
		request.Content = ""
		request.ContentBase64 = base64.StdEncoding.EncodeToString(content)
	} else if len(content) > maxCommandTextBytes {
		g.logger.Warn("the text file is too big to be a command. ", slog.String("path", cmd.File))
		return nil
	}

	if cmd.Replace {
		existing, ok := resolveCommand(g.api.baseURL, token, name, g)
		if ok {
			data, ok := sendCommandRequest(http.MethodPut, g.api.baseURL+"/v1/commands/"+existing.ID.String(), request, token, g)
			if !ok {
				return nil
			}

			fmt.Printf("\nreplaced %s (%s)\n", data.Name, data.ID)
			return nil
		}
	}

	data, ok := sendCommandRequest(http.MethodPost, g.api.baseURL+"/v1/commands", request, token, g)
	if !ok {
		return nil
	}

	fmt.Printf("\nadded %s (%s, %s)\n", data.Name, data.Kind, data.ID)
	fmt.Printf("pull it into a template with: eznit template publish . --command %s=scripts/%s\n", data.ID, data.Name)

	return nil
}

// CommandListCmd lists the caller's library.
type CommandListCmd struct{}

// Run lists the library.
func (cmd *CommandListCmd) Run(g *Globals) error {
	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in. ")
		return nil
	}

	items, ok := sendCommandListRequest(g.api.baseURL, token, g)
	if !ok {
		return nil
	}

	if len(items) == 0 {
		fmt.Println("your command library is empty.")
		return nil
	}

	fmt.Println()

	for i, item := range items {
		marker := " "
		if item.IsExecutable {
			marker = "*"
		}

		fmt.Printf("%3d. %s%s (%s, %d bytes) — %s\n", i+1, marker, item.Name, item.Kind, item.Size, item.ID)
	}

	fmt.Println()

	return nil
}

// CommandShowCmd shows one command.
type CommandShowCmd struct {
	Name string `arg:"" help:"the name or the id of the command"`
}

// Run shows one command.
func (cmd *CommandShowCmd) Run(g *Globals) error {
	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in. ")
		return nil
	}

	item, ok := resolveCommand(g.api.baseURL, token, cmd.Name, g)
	if !ok {
		return nil
	}

	fmt.Println()
	fmt.Printf("%s (%s)\n", item.Name, item.Kind)
	fmt.Printf("id:         %s\n", item.ID)
	fmt.Printf("size:       %d bytes\n", item.Size)
	fmt.Printf("executable: %t\n", item.IsExecutable)

	if item.Kind == commands.CommandKindText {
		fmt.Println()
		fmt.Println(item.Content)
	}

	fmt.Println()

	return nil
}

// CommandRemoveCmd removes a command from the library.
type CommandRemoveCmd struct {
	Name string `arg:"" help:"the name or the id of the command"`
}

// Run removes the command.
func (cmd *CommandRemoveCmd) Run(g *Globals) error {
	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in. ")
		return nil
	}

	item, ok := resolveCommand(g.api.baseURL, token, cmd.Name, g)
	if !ok {
		return nil
	}

	endpoint := g.api.baseURL + "/v1/commands/" + item.ID.String()

	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		g.logger.Warn("failed to send request to " + endpoint)
		return nil
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		g.logger.Warn("request failed. ", slog.String("error", err.Error()))
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var envelope types.Envelope[any]
		_ = json.NewDecoder(resp.Body).Decode(&envelope)

		if resp.StatusCode == http.StatusConflict {
			g.logger.Warn("the command is pulled into a template; update the template first. ")
			return nil
		}

		g.logger.Warn("the command could not be removed. ", slog.Any("result", envelope))
		return nil
	}

	fmt.Printf("\nremoved %s\n", item.Name)

	return nil
}

// sendCommandListRequest asks the API for the caller's library.
func sendCommandListRequest(baseURL string, token string, g *Globals) ([]commands.CommandData, bool) {
	endpoint := baseURL + "/v1/commands"

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		g.logger.Warn("failed to send request to " + endpoint)
		return nil, false
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		g.logger.Warn("request failed. ", slog.String("error", err.Error()))
		return nil, false
	}
	defer resp.Body.Close()

	var envelope types.Envelope[commands.ListResponse]

	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		g.logger.Error("failed to decode. ", slog.String("error", err.Error()))
		return nil, false
	}

	if resp.StatusCode != http.StatusOK {
		g.logger.Warn("the commands could not be read. ", slog.Any("result", envelope))
		return nil, false
	}

	return envelope.Data.Data, true
}

// resolveCommand finds one command by id or by name.
func resolveCommand(baseURL string, token string, nameOrID string, g *Globals) (*commands.CommandData, bool) {
	query := strings.TrimSpace(nameOrID)
	if query == "" {
		g.logger.Warn("a command needs a name or an id. ")
		return nil, false
	}

	if id, err := uuid.Parse(query); err == nil {
		endpoint := baseURL + "/v1/commands/" + id.String()

		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			g.logger.Warn("failed to send request to " + endpoint)
			return nil, false
		}

		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			g.logger.Warn("request failed. ", slog.String("error", err.Error()))
			return nil, false
		}
		defer resp.Body.Close()

		var envelope types.Envelope[commands.CommandData]
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			g.logger.Error("failed to decode. ", slog.String("error", err.Error()))
			return nil, false
		}

		if resp.StatusCode != http.StatusOK {
			g.logger.Warn("the command could not be read. ", slog.Any("result", envelope))
			return nil, false
		}

		return &envelope.Data, true
	}

	items, ok := sendCommandListRequest(baseURL, token, g)
	if !ok {
		return nil, false
	}

	for i := range items {
		if items[i].Name == query {
			return &items[i], true
		}
	}

	g.logger.Warn("no command with that name. ", slog.String("name", query))

	return nil, false
}

// sendCommandRequest creates or replaces a command.
func sendCommandRequest(method string, endpoint string, request commands.CommandRequest, token string, g *Globals) (*commands.CommandData, bool) {
	body, err := json.Marshal(request)
	if err != nil {
		g.logger.Error("failed to encode the command. ", slog.String("error", err.Error()))
		return nil, false
	}

	req, err := http.NewRequest(method, endpoint, bytes.NewReader(body))
	if err != nil {
		g.logger.Warn("failed to send request to " + endpoint)
		return nil, false
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		g.logger.Warn("request failed. ", slog.String("error", err.Error()))
		return nil, false
	}
	defer resp.Body.Close()

	var envelope types.Envelope[commands.CommandData]
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		g.logger.Error("failed to decode. ", slog.String("error", err.Error()))
		return nil, false
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		g.logger.Warn("the command could not be stored. ", slog.Any("result", envelope))
		return nil, false
	}

	return &envelope.Data, true
}

// parseCommandSpec reads a template command reference: `<id>=<path>`.
func parseCommandSpec(spec string) (uuid.UUID, string, bool) {
	idPart, pathPart, ok := strings.Cut(strings.TrimSpace(spec), "=")
	if !ok {
		return uuid.Nil, "", false
	}

	id, err := uuid.Parse(strings.TrimSpace(idPart))
	if err != nil {
		return uuid.Nil, "", false
	}

	targetPath := strings.TrimSpace(pathPart)
	if targetPath == "" {
		return uuid.Nil, "", false
	}

	return id, targetPath, true
}
