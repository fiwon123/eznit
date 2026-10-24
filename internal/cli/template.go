package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/pkg/types"
	"github.com/google/uuid"
)

// Template Cmd works with the templates: the catalogue a user browses, and
// the folder structures a user publishes and applies.
type TemplateCmd struct {
	List    TemplateListCmd    `cmd:"" help:"list the templates in the catalogue"`
	Show    TemplateShowCmd    `cmd:"" help:"show a template, its files and its variables"`
	Publish TemplatePublishCmd `cmd:"" help:"publish a folder as a template"`
	Apply   TemplateApplyCmd   `cmd:"" help:"apply a template to a folder"`
	Init    TemplateInitCmd    `cmd:"" help:"write a starter structure locally, no server needed"`
}

// Template List Cmd shows the catalogue: the public templates and the ones
// the user published.
type TemplateListCmd struct {
	Kind string `help:"only the templates for this terminal" optional:""`
	Mine bool   `help:"only the templates you published"`
}

// Template Show Cmd shows one template: its files and the variables it asks
// for.
type TemplateShowCmd struct {
	ID string `arg:"" optional:"" help:"the id of the template"`
}

// Run lists the catalogue.
func (cmd *TemplateListCmd) Run(g *Globals) error {
	// The catalogue is public: a machine that has never logged in reads the
	// published templates. `--mine` is not — it answers the caller's own
	// templates, private ones included — so that flag still needs a session.
	token, _ := getToken()
	if cmd.Mine && token == "" {
		g.logger.Warn("not logged in. ")
		return nil
	}

	kind := strings.TrimSpace(cmd.Kind)
	if kind != "" && !templates.IsKind(kind) {
		g.logger.Warn("that is not a terminal eznit knows. ", slog.String("kind", kind))
		return nil
	}

	items, ok := sendTemplatesRequest(g.api.baseURL, cmd.Mine, kind, token, g)
	if !ok {
		return nil
	}

	// The "mine" route has no kind filter of its own, so the flag is
	// applied here: it always means what it says.
	items = filterTemplatesByKind(items, kind)

	if len(items) == 0 {
		fmt.Println("no template in the catalogue yet.")
		return nil
	}

	fmt.Println()

	for i, item := range items {
		visibility := "private"
		if item.IsPublic {
			visibility = "public"
		}

		fmt.Printf("%3d. %s (%s, %s) — %s\n", i+1, item.Name, item.Kind, visibility, item.ID)
		if item.Description != "" {
			fmt.Printf("     %s\n", item.Description)
		}
	}

	fmt.Println()

	return nil
}

// Run shows one template.
func (cmd *TemplateShowCmd) Run(g *Globals) error {
	// A public template can be read without an account; the token only
	// widens the view to the caller's own private ones.
	token, _ := getToken()

	id := strings.TrimSpace(cmd.ID)

	if id == "" {
		// Without an id the catalogue is shown and the user picks: an id
		// is not something a user remembers.
		items, ok := sendTemplatesRequest(g.api.baseURL, false, "", token, g)
		if !ok {
			return nil
		}

		if len(items) == 0 {
			fmt.Println("no template in the catalogue yet.")
			return nil
		}

		fmt.Println()

		for i, item := range items {
			fmt.Printf("%3d. %s (%s) — %s\n", i+1, item.Name, item.Kind, item.ID)
		}

		fmt.Println()

		choice, ok := promptChoice("template")
		if !ok || choice > len(items) {
			return nil
		}

		id = items[choice-1].ID.String()
	}

	templateID, err := uuid.Parse(id)
	if err != nil {
		g.logger.Warn("that is not a template id. ", slog.String("id", id))
		return nil
	}

	detail, ok := sendTemplateRequest(g.api.baseURL, templateID, token, g)
	if !ok {
		return nil
	}

	printTemplate(detail)

	return nil
}

// printTemplate shows a template: its files and the variables it asks for.
func printTemplate(detail *templates.TemplateDetail) {
	fmt.Println()
	fmt.Printf("%s (%s)\n", detail.Name, detail.Kind)
	fmt.Printf("id:      %s\n", detail.ID)
	fmt.Printf("slug:    %s\n", detail.Slug)
	fmt.Printf("version: %d\n", detail.Version)

	if detail.Description != "" {
		fmt.Printf("\n%s\n", detail.Description)
	}

	fmt.Println()
	fmt.Printf("files (%d):\n", len(detail.Files))
	for _, file := range detail.Files {
		marker := " "
		if file.IsExecutable {
			marker = "*"
		}
		fmt.Printf("  %s %s\n", marker, file.Path)
	}

	if len(detail.Commands) > 0 {
		fmt.Println()
		fmt.Printf("commands (%d):\n", len(detail.Commands))
		for _, command := range detail.Commands {
			marker := " "
			if command.IsExecutable {
				marker = "*"
			}
			fmt.Printf("  %s %s <- %s (%s)\n", marker, command.TargetPath, command.Name, command.CommandID)
		}
	}

	if len(detail.Variables) > 0 {
		fmt.Println()
		fmt.Println("variables:")
		for _, variable := range detail.Variables {
			required := ""
			if variable.Required {
				required = " (required)"
			}
			fmt.Printf("  {{%s}}%s — %s\n", variable.Key, required, variable.Prompt)
		}
	}

	fmt.Println()
	fmt.Printf("apply it with: eznit template apply %s\n", detail.ID)
	fmt.Println()
}

// filterTemplatesByKind keeps the templates of one kind. An empty kind
// keeps them all.
func filterTemplatesByKind(items []templates.TemplateData, kind string) []templates.TemplateData {
	if kind == "" {
		return items
	}

	filtered := make([]templates.TemplateData, 0, len(items))

	for _, item := range items {
		if item.Kind == kind {
			filtered = append(filtered, item)
		}
	}

	return filtered
}

// sendTemplatesRequest asks the API for the catalogue and returns the
// templates in it.
func sendTemplatesRequest(baseURL string, mine bool, kind string, token string, g *Globals) ([]templates.TemplateData, bool) {
	endpoint := baseURL + "/v1/templates"

	if mine {
		endpoint = baseURL + "/v1/templates/mine"
	} else if kind != "" {
		endpoint = endpoint + "?kind=" + url.QueryEscape(kind)
	}

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		g.logger.Warn("failed to send request to " + endpoint)
		return nil, false
	}

	// The catalogue answers without a session; the header goes along only
	// when there is one, so the same request works logged out.
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	g.logger.Debug("request", slog.String("url", endpoint))

	resp, err := client.Do(req)
	if err != nil {
		g.logger.Warn("request failed. ", slog.String("error", err.Error()))
		return nil, false
	}
	defer resp.Body.Close()

	var envelope types.Envelope[templates.ListResponse]

	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		g.logger.Error("failed to decode. ", slog.String("error", err.Error()))
		return nil, false
	}

	if resp.StatusCode != http.StatusOK {
		g.logger.Warn("the templates could not be read. ", slog.Any("result", envelope))
		return nil, false
	}

	return envelope.Data.Data, true
}

// sendTemplateRequest asks the API for one template.
func sendTemplateRequest(baseURL string, id uuid.UUID, token string, g *Globals) (*templates.TemplateDetail, bool) {
	endpoint := baseURL + "/v1/templates/" + id.String()

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		g.logger.Warn("failed to send request to " + endpoint)
		return nil, false
	}

	// A public template is readable without a session; a private one only
	// answers a caller who sends the token that owns it.
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		g.logger.Warn("request failed. ", slog.String("error", err.Error()))
		return nil, false
	}
	defer resp.Body.Close()

	var envelope types.Envelope[templates.TemplateDetail]

	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		g.logger.Error("failed to decode. ", slog.String("error", err.Error()))
		return nil, false
	}

	if resp.StatusCode != http.StatusOK {
		g.logger.Warn("the template could not be read. ", slog.Any("result", envelope))
		return nil, false
	}

	return &envelope.Data, true
}

// promptChoice asks the user to pick one of the numbered items that were
// listed, and returns the number they picked.
//
// The answer comes from the same reader as the other prompts: two readers on
// one stdin lose whatever the first one has buffered.
func promptChoice(what string) (int, bool) {
	fmt.Printf("which %s? (number) ", what)

	answer := promptLine("")

	choice, err := strconv.Atoi(strings.TrimSpace(answer))
	if err != nil || choice < 1 {
		fmt.Println("that is not a number.")
		return 0, false
	}

	return choice, true
}
