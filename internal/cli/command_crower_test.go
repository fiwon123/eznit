package cli

import (
	"strings"
	"testing"

	"github.com/fiwon123/eznit/internal/domain/commands"
	"github.com/fiwon123/eznit/internal/interop/crower"
)

func TestCrowerPackFrom(t *testing.T) {
	pack, skipped := crowerPackFrom([]commands.CommandData{
		{Name: "fmt", Kind: commands.CommandKindText, Content: "#!/bin/sh\ngofmt -w .\n", IsExecutable: true},
		{Name: "helper", Kind: commands.CommandKindBinary, Size: 1024},
		{Name: "lint", Kind: commands.CommandKindText, Content: "golangci-lint run\n"},
	})

	if len(pack) != 2 {
		t.Fatalf("pack = %d commands, want the 2 text ones", len(pack))
	}
	if pack[0].Name != "fmt" || pack[0].Exec != "#!/bin/sh\ngofmt -w .\n" {
		t.Errorf("pack[0] = %+v, want fmt with the script as Exec", pack[0])
	}
	if len(skipped) != 1 || skipped[0] != "helper" {
		t.Errorf("skipped = %v, want the binary command named", skipped)
	}
}

func TestCrowerRequestsFrom(t *testing.T) {
	tests := []struct {
		name      string
		command   crower.Command
		wantName  string
		wantBody  string
		wantNotes int
		wantSkip  bool
	}{
		{
			name:     "exec becomes the script",
			command:  crower.Command{Name: "fmt", Exec: "gofmt -w ."},
			wantName: "fmt",
			wantBody: "gofmt -w .",
		},
		{
			name:     "steps win over exec",
			command:  crower.Command{Name: "deploy", Exec: "ignored", Steps: []string{"git pull", "systemctl restart app"}},
			wantName: "deploy",
			wantBody: "git pull\nsystemctl restart app",
		},
		{
			name:      "aliases are reported",
			command:   crower.Command{Name: "backup", Exec: "tar czf b.tgz .", Aliases: []string{"bkp", "b"}},
			wantName:  "backup",
			wantBody:  "tar czf b.tgz .",
			wantNotes: 1,
		},
		{
			name:      "nothing to run",
			command:   crower.Command{Name: "empty", Steps: []string{"  "}},
			wantSkip:  true,
			wantNotes: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests, notes := crowerRequestsFrom([]crower.Command{tt.command})

			if tt.wantSkip {
				if len(requests) != 0 {
					t.Fatalf("requests = %d, want none", len(requests))
				}
			} else {
				if len(requests) != 1 {
					t.Fatalf("requests = %d, want 1", len(requests))
				}
				if requests[0].Name != tt.wantName || requests[0].Content != tt.wantBody {
					t.Errorf("request = %+v, want name %q and body %q", requests[0], tt.wantName, tt.wantBody)
				}
				if requests[0].IsExecutable == nil || !*requests[0].IsExecutable {
					t.Error("IsExecutable = false, want a crower command to stay executable")
				}
			}

			if len(notes) != tt.wantNotes {
				t.Errorf("notes = %v, want %d", notes, tt.wantNotes)
			}
		})
	}
}

func TestCrowerRequestsFromRefusesOversize(t *testing.T) {
	requests, notes := crowerRequestsFrom([]crower.Command{{
		Name: "huge",
		Exec: strings.Repeat("a", maxCommandTextBytes+1),
	}})

	if len(requests) != 0 {
		t.Fatalf("requests = %d, want the oversize command skipped", len(requests))
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "larger") {
		t.Errorf("notes = %v, want one note naming the size limit", notes)
	}
}

// A pack written by export and read back produces the same requests: the
// round trip through the file does not change the names or the bodies.
func TestCrowerPackRoundTrip(t *testing.T) {
	original := []commands.CommandData{
		{Name: "fmt", Kind: commands.CommandKindText, Content: "gofmt -w ."},
		{Name: "deploy", Kind: commands.CommandKindText, Content: "git pull"},
	}

	pack, _ := crowerPackFrom(original)
	requests, _ := crowerRequestsFrom(pack)

	if len(requests) != len(original) {
		t.Fatalf("requests = %d, want %d", len(requests), len(original))
	}

	for i, request := range requests {
		if request.Name != original[i].Name || request.Content != original[i].Content {
			t.Errorf("request[%d] = %+v, want %+v", i, request, original[i])
		}
	}
}
