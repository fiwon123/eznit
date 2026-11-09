package cli

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/pkg/logger"
)

func bundleTestGlobals(t *testing.T) *Globals {
	t.Helper()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}

	return newGlobals(&API{}, "", log)
}

// answerWith feeds the prompts in a test and restores the reader afterwards.
func answerWith(t *testing.T, input string) {
	t.Helper()

	previous := stdinReader
	stdinReader = bufio.NewReader(strings.NewReader(input))
	t.Cleanup(func() { stdinReader = previous })
}

func writeBundleFile(t *testing.T, bundle *templates.Bundle) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "starter.eznit")

	var buffer bytes.Buffer
	if err := templates.WriteBundle(&buffer, bundle); err != nil {
		t.Fatalf("WriteBundle() error = %v", err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0644); err != nil {
		t.Fatalf("write the bundle: %v", err)
	}

	return path
}

func sampleBundle() *templates.Bundle {
	return &templates.Bundle{
		Format: templates.BundleFormat,
		Template: templates.BundleTemplate{
			Name: "Starter",
			Kind: templates.KindGeneric,
		},
		Variables: []templates.BundleVariable{
			{Key: "PROJECT_NAME", Prompt: "name?", DefaultValue: "demo"},
		},
		Files: []templates.BundleFile{
			{Path: "AGENTS.md", Kind: templates.FileKindText, Content: "# {{PROJECT_NAME}}\n"},
			{
				Path:          "bin/run",
				Kind:          templates.FileKindBinary,
				ContentBase64: base64.StdEncoding.EncodeToString([]byte{0x00, 0x01, 0xff}),
				IsExecutable:  true,
			},
		},
	}
}

// Applying a bundle writes the tree with no server and no session: the
// variable is answered (or defaulted) locally, and the binary is decoded.
func TestApplyFromBundleWritesTheTree(t *testing.T) {
	answerWith(t, "\n") // accept the default for PROJECT_NAME

	path := writeBundleFile(t, sampleBundle())
	target := t.TempDir()

	cmd := &TemplateApplyCmd{FromFile: path, Target: target}
	if err := cmd.Run(bundleTestGlobals(t)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	agents, err := os.ReadFile(filepath.Join(target, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if string(agents) != "# demo\n" {
		t.Errorf("AGENTS.md = %q, want the default substituted", agents)
	}

	info, err := os.Stat(filepath.Join(target, "bin", "run"))
	if err != nil {
		t.Fatalf("stat bin/run: %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Errorf("bin/run mode = %v, want the executable bit", info.Mode().Perm())
	}

	binary, err := os.ReadFile(filepath.Join(target, "bin", "run"))
	if err != nil {
		t.Fatalf("read bin/run: %v", err)
	}
	if !bytes.Equal(binary, []byte{0x00, 0x01, 0xff}) {
		t.Errorf("bin/run = %v, want the original bytes", binary)
	}
}

// Without --force, an existing file stops the apply before anything is
// written; with it, the file is replaced.
func TestApplyFromBundleRespectsForce(t *testing.T) {
	answerWith(t, "\n")

	path := writeBundleFile(t, sampleBundle())
	target := t.TempDir()

	existing := filepath.Join(target, "AGENTS.md")
	if err := os.WriteFile(existing, []byte("mine"), 0644); err != nil {
		t.Fatalf("seed AGENTS.md: %v", err)
	}

	cmd := &TemplateApplyCmd{FromFile: path, Target: target}
	if err := cmd.Run(bundleTestGlobals(t)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if content, err := os.ReadFile(existing); err != nil || string(content) != "mine" {
		t.Fatalf("AGENTS.md = %q, %v, want the existing file untouched", content, err)
	}

	answerWith(t, "\n")
	cmd = &TemplateApplyCmd{FromFile: path, Target: target, Force: true}
	if err := cmd.Run(bundleTestGlobals(t)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if content, _ := os.ReadFile(existing); string(content) != "# demo\n" {
		t.Errorf("AGENTS.md = %q, want the bundle written over it", content)
	}
}

// A template id and --from-file are two different sources; naming both is a
// mistake, and nothing should be written.
func TestApplyRefusesIDAndBundleTogether(t *testing.T) {
	path := writeBundleFile(t, sampleBundle())
	target := t.TempDir()

	cmd := &TemplateApplyCmd{ID: "00000000-0000-0000-0000-000000000000", FromFile: path, Target: target}
	if err := cmd.Run(bundleTestGlobals(t)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("target holds %d entries, want none", len(entries))
	}
}

func TestFilesFromBundleDecodesBinary(t *testing.T) {
	files, err := filesFromBundle(sampleBundle())
	if err != nil {
		t.Fatalf("filesFromBundle() error = %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
	if files[1].Kind != templates.FileKindBinary || !bytes.Equal(files[1].Bytes, []byte{0x00, 0x01, 0xff}) {
		t.Errorf("binary file = %+v, want the decoded bytes", files[1])
	}
}
