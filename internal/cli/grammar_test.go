package cli

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// TestTemplatePublishAcceptsTheCommandsFlag parses the command line the
// `command add` hint prints. Those words are copy-pasted by users, so a rename
// of the flag (it once read `--command`, which kong rejects) must fail here.
func TestTemplatePublishAcceptsTheCommandsFlag(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("failed to build the parser: %v", err)
	}

	ctx, err := parser.Parse([]string{
		"template", "publish", ".", "--commands", "cmdid=scripts/fmt.sh",
	})
	if err != nil {
		t.Fatalf("the documented --commands flag should parse: %v", err)
	}
	if !strings.HasPrefix(ctx.Command(), "template publish") {
		t.Fatalf("parsed command = %q, want it to start with %q", ctx.Command(), "template publish")
	}

	got := cli.Template.Publish.Commands
	if len(got) != 1 || got[0] != "cmdid=scripts/fmt.sh" {
		t.Fatalf("Commands = %v, want [cmdid=scripts/fmt.sh]", got)
	}
	if cli.Template.Publish.Folder != "." {
		t.Fatalf("Folder = %q, want the default %q", cli.Template.Publish.Folder, ".")
	}
}
