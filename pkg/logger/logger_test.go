package logger

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withoutStdout sends the console core's output to a pipe that is drained and
// discarded. The cores capture os.Stdout when they are built, so the swap has
// to happen before NewJson/NewConsole is called. Tests that touch the console
// must not run in parallel — os.Stdout is process-wide.
func withoutStdout(t *testing.T) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	t.Cleanup(func() {
		_ = w.Close()
		os.Stdout = old
		_, _ = io.Copy(io.Discard, r)
		_ = r.Close()
	})
}

// readLog returns the non-empty lines of the file the logger writes, failing
// the test when the file was never created.
func readLog(t *testing.T, dir string) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, "current.log"))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}

	var lines []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// decodeJSONLog parses one line of the JSON logger. A line that is not a JSON
// object is a failure, not something to skip: the file format is the contract
// a log collector reads.
func decodeJSONLog(t *testing.T, line string) map[string]any {
	t.Helper()

	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("log line is not a JSON object: %v\nline: %s", err, line)
	}
	return entry
}

func TestNewJsonWritesJSONLines(t *testing.T) {
	withoutStdout(t)
	dir := t.TempDir()

	cfg, err := NewJson(dir, false)
	if err != nil {
		t.Fatalf("NewJson() error = %v", err)
	}
	cfg.Info("created file", "id", 42)
	cfg.Sync()

	lines := readLog(t, dir)
	if len(lines) != 1 {
		t.Fatalf("read %d log lines, want 1: %v", len(lines), lines)
	}

	entry := decodeJSONLog(t, lines[0])
	if got := entry["msg"]; got != "created file" {
		t.Errorf("msg = %v, want %q", got, "created file")
	}
	if got := entry["level"]; got != "INFO" {
		t.Errorf("level = %v, want %q", got, "INFO")
	}
	if got := entry["id"]; got != float64(42) {
		t.Errorf("id = %v, want 42", got)
	}

	ts, ok := entry["ts"].(string)
	if !ok {
		t.Fatalf("ts = %v, want a string", entry["ts"])
	}
	if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
		t.Errorf("ts = %q is not RFC3339Nano: %v", ts, err)
	}
}

// The default level is Info: a Debug entry must not reach the file, or a
// production log fills with lines nobody asked for.
func TestNewJsonFiltersDebugAtInfoLevel(t *testing.T) {
	withoutStdout(t)
	dir := t.TempDir()

	cfg, err := NewJson(dir, false)
	if err != nil {
		t.Fatalf("NewJson() error = %v", err)
	}
	cfg.Debug("filtered out")
	cfg.Info("kept")
	cfg.Sync()

	for _, line := range readLog(t, dir) {
		if entry := decodeJSONLog(t, line); entry["msg"] == "filtered out" {
			t.Errorf("a Debug entry reached the file at Info level: %s", line)
		}
	}
}

func TestNewJsonDebugLevelWritesDebug(t *testing.T) {
	withoutStdout(t)
	dir := t.TempDir()

	cfg, err := NewJson(dir, true)
	if err != nil {
		t.Fatalf("NewJson() error = %v", err)
	}
	cfg.Debug("debug entry")
	cfg.Sync()

	var found bool
	for _, line := range readLog(t, dir) {
		entry := decodeJSONLog(t, line)
		if entry["msg"] == "debug entry" {
			found = true
			if got := entry["level"]; got != "DEBUG" {
				t.Errorf("level = %v, want %q", got, "DEBUG")
			}
		}
	}
	if !found {
		t.Error("the Debug entry is missing from the file at debug level")
	}
}

// NewConsole with onlyLogFile skips the console core entirely, which is what
// the CLI uses when its stdout carries data rather than logs.
func TestNewConsoleOnlyLogFile(t *testing.T) {
	withoutStdout(t)
	dir := t.TempDir()

	cfg, err := NewConsole(dir, true, true)
	if err != nil {
		t.Fatalf("NewConsole() error = %v", err)
	}
	cfg.Debug("console debug entry")
	cfg.Sync()

	lines := readLog(t, dir)
	if len(lines) == 0 {
		t.Fatal("the log file is empty")
	}

	var found bool
	for _, line := range lines {
		if strings.Contains(line, "console debug entry") && strings.Contains(line, "DEBUG") {
			found = true
		}
	}
	if !found {
		t.Errorf("the entry is missing from the console-encoder file: %v", lines)
	}
}

func TestEmptyLogFolderIsAnError(t *testing.T) {
	withoutStdout(t)

	if _, err := NewJson("", false); err == nil {
		t.Error("NewJson(\"\") error = nil, want an error")
	}
	if _, err := NewConsole("", false, true); err == nil {
		t.Error("NewConsole(\"\", onlyLogFile) error = nil, want an error")
	}
	if _, err := NewConsole("", false, false); err == nil {
		t.Error("NewConsole(\"\") error = nil, want an error")
	}
}

// Sync may be called more than once (deferred and explicit are both common)
// and must stay a no-op after the first call rather than panicking.
func TestSyncIsIdempotent(t *testing.T) {
	withoutStdout(t)

	cfg, err := NewJson(t.TempDir(), false)
	if err != nil {
		t.Fatalf("NewJson() error = %v", err)
	}

	cfg.Sync()
	cfg.Sync()
}
