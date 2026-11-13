package cli

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/fiwon123/eznit/internal/update"
)

// buildVersion is what this binary was built as; Run hands it in. The update
// command compares against it and records it as the version an install
// replaces.
var buildVersion = "dev"

// UpdateCmd installs a release, checks for one, or restores the previous one.
type UpdateCmd struct {
	Check          bool   `help:"only report whether a newer release exists"`
	InstallVersion string `help:"install this release tag, including an older one" name:"install-version"`
	Revert         bool   `help:"put back the version saved by the last install"`
	Yes            bool   `short:"y" help:"do not ask for confirmation"`
}

// Run updates the CLI. The desktop app has the same controls in About.
func (cmd *UpdateCmd) Run(g *Globals) error {
	updater := update.New(update.MemberCLI)

	switch {
	case cmd.Revert:
		previous, ok := updater.Previous()
		if !ok {
			g.logger.Warn("no previous version is recorded")
			return nil
		}

		if !cmd.Yes && !confirm(fmt.Sprintf("Restore eznit %s?", previous), g) {
			return nil
		}

		if err := updater.Restore(); err != nil {
			g.logger.Warn("the previous version was not restored. ", slog.String("error", err.Error()))
			return nil
		}

		fmt.Printf("\nrestored eznit %s\n", previous)
		return nil

	case cmd.Check:
		latest, err := updater.Latest()
		if err != nil {
			g.logger.Warn("the release could not be checked. ", slog.String("error", err.Error()))
			return nil
		}

		if latest == buildVersion {
			fmt.Printf("\neznit %s is the newest release\n", buildVersion)
			return nil
		}

		fmt.Printf("\neznit %s is available (you have %s)\n", latest, buildVersion)
		return nil
	}

	target := strings.TrimSpace(cmd.InstallVersion)
	if target == "" {
		latest, err := updater.Latest()
		if err != nil {
			g.logger.Warn("the release could not be checked. ", slog.String("error", err.Error()))
			return nil
		}

		if latest == buildVersion {
			fmt.Printf("\neznit %s is the newest release\n", buildVersion)
			return nil
		}

		target = latest
	}

	if !cmd.Yes && !confirm(fmt.Sprintf("Install eznit %s? It replaces the binary on disk.", target), g) {
		return nil
	}

	if err := updater.Install(buildVersion, target); err != nil {
		g.logger.Warn("the update failed. ", slog.String("error", err.Error()))
		return nil
	}

	fmt.Printf("\ninstalled eznit %s — run it again to use the new version\n", target)
	return nil
}

// confirm asks a yes/no question. Declining is a choice, not a failure.
func confirm(question string, g *Globals) bool {
	fmt.Print(question + " [y/N]: ")

	answer := strings.ToLower(strings.TrimSpace(promptLine("")))

	return answer == "y" || answer == "yes"
}
