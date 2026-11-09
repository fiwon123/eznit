package cli

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/fiwon123/eznit/internal/domain/commands"
	"github.com/fiwon123/eznit/internal/interop/crower"
)

// Command Export Cmd writes the command library as a crower pack, and Command
// Import Cmd reads one back. The pack is crower's TOML — see
// internal/interop/crower for the format and the limits of the conversion.
type CommandExportCmd struct {
	Crower string `help:"write the library to a crower TOML pack" required:"" placeholder:"FILE"`
	Force  bool   `help:"overwrite the pack if it is already there"`
}

// Run writes the pack.
func (cmd *CommandExportCmd) Run(g *Globals) error {
	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in. ")
		return nil
	}

	items, ok := sendCommandListRequest(g.api.baseURL, token, g)
	if !ok {
		return nil
	}

	pack, skipped := crowerPackFrom(items)

	if len(pack) == 0 {
		g.logger.Warn("the library holds no text command to export. ")
		for _, name := range skipped {
			fmt.Printf("skipped %s (a binary command has no crower equivalent)\n", name)
		}
		return nil
	}

	path := strings.TrimSpace(cmd.Crower)
	if !cmd.Force {
		if _, err := os.Stat(path); err == nil {
			g.logger.Warn("the file is already there; use --force to replace it. ", slog.String("path", path))
			return nil
		}
	}

	if err := crower.WriteFile(path, pack); err != nil {
		g.logger.Warn("the pack was not written. ", slog.String("error", err.Error()))
		return nil
	}

	for _, name := range skipped {
		fmt.Printf("skipped %s (a binary command has no crower equivalent)\n", name)
	}

	fmt.Println()
	fmt.Printf("exported %d commands to %s\n", len(pack), path)
	fmt.Println()

	return nil
}

// Command Import Cmd reads a crower pack into the library.
type CommandImportCmd struct {
	Crower  string `help:"read commands from a crower TOML pack" required:"" placeholder:"FILE"`
	Replace bool   `help:"replace a command that is already in the library"`
}

// Run imports the pack.
func (cmd *CommandImportCmd) Run(g *Globals) error {
	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in. ")
		return nil
	}

	pack, err := crower.ReadFile(strings.TrimSpace(cmd.Crower))
	if err != nil {
		g.logger.Warn("the pack was not read. ", slog.String("error", err.Error()))
		return nil
	}

	requests, notes := crowerRequestsFrom(pack)

	existing, ok := sendCommandListRequest(g.api.baseURL, token, g)
	if !ok {
		return nil
	}

	byName := make(map[string]*commands.CommandData, len(existing))
	for i := range existing {
		byName[existing[i].Name] = &existing[i]
	}

	added, replaced, kept := 0, 0, 0

	for _, request := range requests {
		if current, exists := byName[request.Name]; exists {
			if !cmd.Replace {
				fmt.Printf("kept %s (already there; --replace to update it)\n", request.Name)
				kept++
				continue
			}

			endpoint := g.api.baseURL + "/v1/commands/" + current.ID.String()
			if _, ok := sendCommandRequest(http.MethodPut, endpoint, request, token, g); !ok {
				kept++
				continue
			}
			replaced++
			continue
		}

		if _, ok := sendCommandRequest(http.MethodPost, g.api.baseURL+"/v1/commands", request, token, g); !ok {
			kept++
			continue
		}
		added++
	}

	for _, note := range notes {
		fmt.Printf("note: %s\n", note)
	}

	fmt.Println()
	fmt.Printf("imported from %s: %d added, %d replaced, %d kept\n", cmd.Crower, added, replaced, kept)
	fmt.Println()

	return nil
}

// crowerPackFrom turns the text commands of a library into a pack. A binary
// command has no crower equivalent — crower runs a line or a list of lines —
// so it is named rather than silently dropped.
func crowerPackFrom(items []commands.CommandData) ([]crower.Command, []string) {
	pack := make([]crower.Command, 0, len(items))
	var skipped []string

	for _, item := range items {
		if item.Kind == commands.CommandKindBinary {
			skipped = append(skipped, item.Name)
			continue
		}

		// The script becomes the line crower runs. An executable flag has
		// no crower field: what runs is the line, through a shell.
		pack = append(pack, crower.Command{
			Name: item.Name,
			Exec: item.Content,
		})
	}

	return pack, skipped
}

// crowerRequestsFrom turns a pack into the requests that create the commands.
// It returns the notes the user should see: a stepless entry and an aliased
// one both lose something, and silence about that is a bug.
func crowerRequestsFrom(pack []crower.Command) ([]commands.CommandRequest, []string) {
	requests := make([]commands.CommandRequest, 0, len(pack))
	var notes []string

	for _, command := range pack {
		// crower runs Steps instead of Exec when Steps is present.
		content := command.Exec
		if len(command.Steps) > 0 {
			content = strings.Join(command.Steps, "\n")
		}

		if strings.TrimSpace(content) == "" {
			notes = append(notes, fmt.Sprintf("%s has no line to run; skipped", command.Name))
			continue
		}

		if len(content) > maxCommandTextBytes {
			notes = append(notes, fmt.Sprintf("%s is larger than a text command allows; skipped", command.Name))
			continue
		}

		// A crower command is something you run, and eznit keeps the bit
		// so a template can write it as executable.
		executable := true

		requests = append(requests, commands.CommandRequest{
			Name:         command.Name,
			Kind:         commands.CommandKindText,
			Content:      content,
			IsExecutable: &executable,
		})

		if len(command.Aliases) > 0 {
			notes = append(notes, fmt.Sprintf("%s: the aliases %s are not kept, eznit has one name per command",
				command.Name, strings.Join(command.Aliases, ", ")))
		}
	}

	return requests, notes
}
