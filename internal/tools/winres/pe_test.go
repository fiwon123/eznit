package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildFixture compiles a minimal Windows program, so the checks run against a
// real image the Windows loader would accept rather than a hand-made header.
func buildFixture(t *testing.T, goarch string, gui bool) string {
	t.Helper()

	dir := t.TempDir()

	files := map[string]string{
		"go.mod":  "module fixture\n\ngo 1.25\n",
		"main.go": "package main\n\nfunc main() {}\n",
	}

	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	output := filepath.Join(dir, "probe.exe")

	args := []string{"build", "-o", output}
	if gui {
		args = append(args, "-ldflags", "-H windowsgui")
	}

	args = append(args, ".")

	command := exec.Command("go", args...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GOOS=windows", "GOARCH="+goarch, "CGO_ENABLED=0")

	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build fixture for %s: %v\n%s", goarch, err, out)
	}

	return output
}

func inspectFile(t *testing.T, path string) Info {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()

	info, err := Inspect(file)
	if err != nil {
		t.Fatalf("Inspect %s: %v", path, err)
	}

	return info
}

// The header says the machine, the subsystem and — when there is one — the
// resource directory.
func TestInspectReadsTheHeaders(t *testing.T) {
	info := inspectFile(t, buildFixture(t, "amd64", false))

	if info.Machine != MachineAMD64 {
		t.Errorf("machine = %#x, want amd64", info.Machine)
	}

	if info.Subsystem != SubsystemConsole {
		t.Errorf("subsystem = %d, want console", info.Subsystem)
	}

	if len(info.Resources) != 0 {
		t.Errorf("resources = %v, want none in a bare build", info.ResourceList())
	}

	if err := info.VerifyConsole(); err != nil {
		t.Errorf("VerifyConsole: %v", err)
	}

	if err := info.VerifyMachine(MachineAMD64); err != nil {
		t.Errorf("VerifyMachine(amd64): %v", err)
	}

	if err := info.VerifyMachine(MachineARM64); err == nil {
		t.Error("an amd64 image passed as arm64")
	}
}

// A window executable with no resource directory is exactly the regression #456
// fixed, and the check has to name it.
func TestVerifyGUIRejectsABareWindowBinary(t *testing.T) {
	info := inspectFile(t, buildFixture(t, "amd64", true))

	if info.Subsystem != SubsystemGUI {
		t.Fatalf("subsystem = %d, want GUI", info.Subsystem)
	}

	err := info.VerifyGUI()
	if err == nil {
		t.Fatal("a GUI binary with no resources passed")
	}

	if !strings.Contains(err.Error(), "resource") {
		t.Errorf("error = %q, want it to mention the missing resources", err)
	}

	if err := info.VerifyConsole(); err == nil {
		t.Error("a GUI binary passed as a console program")
	}
}

// The architecture is read from the file name, so an archive can be checked
// against what it claims to be.
func TestMachineForArchive(t *testing.T) {
	tests := []struct {
		archive string
		want    uint16
		windows bool
	}{
		{"mytool_v0.20.16_windows_amd64.zip", MachineAMD64, true},
		{"mytool_v0.20.16_windows_arm64.zip", MachineARM64, true},
		{"/tmp/build/dist/mytool_v1.0.0_windows_386.zip", MachineI386, true},
		{"mytool_v0.20.16_linux_amd64.tar.gz", 0, false},
		{"mytool_v0.20.16_darwin_arm64.tar.gz", 0, false},
	}

	for _, tt := range tests {
		machine, windows := MachineForArchive(tt.archive)

		if windows != tt.windows || machine != tt.want {
			t.Errorf("MachineForArchive(%q) = %#x/%v, want %#x/%v", tt.archive, machine, windows, tt.want, tt.windows)
		}
	}
}

// A file that is not a Windows executable is rejected with an explanation.
func TestInspectRejectsOtherFiles(t *testing.T) {
	if _, err := Inspect(strings.NewReader("MZ but not really")); err == nil {
		t.Error("Inspect accepted a text file")
	}
}
