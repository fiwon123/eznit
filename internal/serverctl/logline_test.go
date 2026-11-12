package serverctl

import (
	"strings"
	"testing"
	"time"
)

// The API's console output is a JSON record per line (pkg/logger.NewJson);
// this is the shape a person has to be able to read.
func TestParseLineReadsAJSONRecord(t *testing.T) {
	line := ParseLine(`{"level":"info","ts":"2026-11-12T14:03:05.123456789Z","msg":"Server Running...","caller":"main.go:110"}`)

	if line.Level != LevelInfo {
		t.Errorf("level = %v, want info", line.Level)
	}
	if line.Message != "Server Running..." {
		t.Errorf("message = %q, want %q", line.Message, "Server Running...")
	}

	want, err := time.Parse(time.RFC3339Nano, "2026-11-12T14:03:05.123456789Z")
	if err != nil {
		t.Fatal(err)
	}
	if !line.At.Equal(want) {
		t.Errorf("At = %v, want %v", line.At, want)
	}
	if len(line.Time) != len("15:04:05") {
		t.Errorf("Time = %q, want an HH:MM:SS rendering of the timestamp", line.Time)
	}

	if len(line.Fields) != 1 || line.Fields[0] != (LogField{Key: "caller", Value: "main.go:110"}) {
		t.Errorf("fields = %v, want one caller field", line.Fields)
	}

	formatted := Format(line)
	if strings.Contains(formatted, `"level"`) || strings.Contains(formatted, `"msg"`) {
		t.Errorf("Format() = %q, want the JSON keys gone", formatted)
	}
	if !strings.Contains(formatted, " INFO ") || !strings.Contains(formatted, "Server Running...") {
		t.Errorf("Format() = %q, want a level column and the message", formatted)
	}
	if !strings.Contains(formatted, "caller=main.go:110") {
		t.Errorf("Format() = %q, want the field as key=value", formatted)
	}
}

// An error keeps its stack, but as a field: a stacktrace is one JSON string
// with newlines in it, and a grid row per newline would break the row/line
// correspondence the window colours by.
func TestParseLineFlattensAnErrorAndItsStack(t *testing.T) {
	line := ParseLine(`{"level":"error","ts":"2026-11-12T14:04:00Z","msg":"Unable to listen on :4000","stacktrace":"goroutine 1\nmain.run()\n\tmain.go:98"}`)

	if line.Level != LevelError {
		t.Errorf("level = %v, want error", line.Level)
	}

	formatted := Format(line)
	if strings.Contains(formatted, "\n") {
		t.Errorf("Format() = %q, want one line", formatted)
	}
	if !strings.Contains(formatted, "goroutine 1 | main.run()") {
		t.Errorf("Format() = %q, want the stack flattened into a field", formatted)
	}
}

// The console encoder colours the level word; the escape sequences must not
// reach the grid as boxes.
func TestParseLineStripsColour(t *testing.T) {
	line := ParseLine("\x1b[31mERROR\x1b[0m\tUnable to connect to PostgreSQL!")

	if line.Level != LevelError {
		t.Errorf("level = %v, want error", line.Level)
	}
	if line.Message != "Unable to connect to PostgreSQL!" {
		t.Errorf("message = %q, want the message without escapes", line.Message)
	}

	for _, text := range []string{line.Message, Format(line)} {
		if strings.ContainsRune(text, '\x1b') {
			t.Errorf("%q still holds an escape sequence", text)
		}
	}
}

// The zap console encoder ends a line with a tab-separated JSON object.
func TestParseLineReadsAConsoleTail(t *testing.T) {
	line := ParseLine("WARN\tSlow request\t{\"duration\":\"1.5s\",\"path\":\"/v1/files\"}")

	if line.Level != LevelWarn {
		t.Errorf("level = %v, want warn", line.Level)
	}
	if line.Message != "Slow request" {
		t.Errorf("message = %q, want %q", line.Message, "Slow request")
	}
	if len(line.Fields) != 2 || line.Fields[0].Key != "duration" || line.Fields[0].Value != "1.5s" {
		t.Errorf("fields = %v, want duration then path", line.Fields)
	}
}

// A line that is not a record must survive whole: a panic is the reason a
// start failed, and dropping it would hide that.
func TestParseLineKeepsAPlainLine(t *testing.T) {
	const panicLine = "panic: runtime error: invalid memory address or nil pointer dereference"

	line := ParseLine(panicLine)

	if line.Level != LevelOther {
		t.Errorf("level = %v, want other", line.Level)
	}
	if line.Message != "" {
		t.Errorf("message = %q, want the raw line to be the message of last resort", line.Message)
	}
	if Format(line) != "  "+panicLine {
		t.Errorf("Format() = %q, want the line indented and unchanged", Format(line))
	}
}

// A JSON object that is not a log record stays text: reducing it to fields
// would lose the object itself.
func TestParseLineKeepsUnknownJSONAsText(t *testing.T) {
	const raw = `{"api":"v1","status":200}`

	line := ParseLine(raw)

	if line.Level != LevelOther || len(line.Fields) != 0 {
		t.Errorf("line = %+v, want it treated as text", line)
	}
	if !strings.Contains(Format(line), raw) {
		t.Errorf("Format() = %q, want the object unchanged", Format(line))
	}
}

// Format aligns the level column so a person can scan a wall of lines.
func TestFormatAlignsTheLevelColumn(t *testing.T) {
	info := Format(LogLine{Time: "14:03:05", Level: LevelInfo, Message: "one"})
	warn := Format(LogLine{Time: "14:03:06", Level: LevelWarn, Message: "two"})
	debug := Format(LogLine{Time: "14:03:07", Level: LevelDebug, Message: "three"})

	if strings.Index(info, "one") != strings.Index(warn, "two") || strings.Index(info, "one") != strings.Index(debug, "three") {
		t.Errorf("messages do not line up:\n%q\n%q\n%q", info, warn, debug)
	}
}

// A blank line is not a record either, and must not panic the formatter.
func TestParseLineHandlesBlankLines(t *testing.T) {
	line := ParseLine("   \r\n")

	if line.Level != LevelOther {
		t.Errorf("level = %v, want other", line.Level)
	}
	if got := Format(line); got != "" {
		t.Errorf("Format() = %q, want nothing to draw", got)
	}
}
