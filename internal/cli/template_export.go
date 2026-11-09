package cli

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/fiwon123/eznit/internal/domain/templates"
)

// Template Export Cmd writes a template to one file, so it can be applied on a
// machine that has no server and no account:
//
//	eznit template export <id> -o starter.eznit
//	eznit template apply --from-file starter.eznit
//
// A private template needs a session to export (the API hides it otherwise); a
// public one does not.
type TemplateExportCmd struct {
	ID     string `arg:"" optional:"" help:"the id of the template"`
	Output string `short:"o" help:"the file to write" default:"template.eznit"`
	Force  bool   `help:"overwrite the file if it is already there"`
}

// Run writes the bundle.
func (cmd *TemplateExportCmd) Run(g *Globals) error {
	token, _ := getToken()

	detail, ok := findTemplate(g, cmd.ID, token)
	if !ok {
		return nil
	}

	bundle, err := templates.NewBundle(detail)
	if err != nil {
		g.logger.Warn("the template was not exported. ", slog.String("error", err.Error()))
		return nil
	}

	path := strings.TrimSpace(cmd.Output)
	if path == "" {
		path = "template.eznit"
	}

	if !cmd.Force {
		if _, err := os.Stat(path); err == nil {
			g.logger.Warn("the file is already there; use --force to replace it. ", slog.String("path", path))
			return nil
		}
	}

	// The bundle is built in memory first: a failed write leaves no
	// half-written file for apply to read.
	var buffer bytes.Buffer
	if err := templates.WriteBundle(&buffer, bundle); err != nil {
		g.logger.Warn("the template was not exported. ", slog.String("error", err.Error()))
		return nil
	}

	if err := os.WriteFile(path, buffer.Bytes(), 0644); err != nil {
		g.logger.Warn("the bundle was not written. ", slog.String("error", err.Error()))
		return nil
	}

	fmt.Println()
	fmt.Printf("exported %s to %s (%d files)\n", bundle.Template.Name, path, len(bundle.Files))
	fmt.Println()

	return nil
}
