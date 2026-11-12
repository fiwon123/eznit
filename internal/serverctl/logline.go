package serverctl

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// Level is how much attention a line of the child's output wants, so a window
// can colour it without understanding the logger that wrote it.
type Level int

const (
	// LevelOther is a line that is not a log record: a panic, a plain message
	// from a tool the server started, or a blank line.
	LevelOther Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

// String is the word the server's own log uses, or "" for a line that has
// none. A window pads it to a fixed column.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	}

	return ""
}

// LogLine is one line of the child's output, split into the parts a person
// reads: when, how bad, what, and the key/value tail. It is never more than
// one line — a message that carried newlines is flattened — so a grid of them
// keeps one row per line of output.
type LogLine struct {
	At      time.Time // zero when the line carries no parseable timestamp
	Time    string    // At in the reader's zone as HH:MM:SS, empty when At is zero
	Level   Level
	Message string
	Fields  []LogField
	Raw     string
}

// LogField is one structured value the server attached, in display order.
type LogField struct {
	Key   string
	Value string
}

// The keys a structured log record uses for the parts that are not values.
// `stacktrace` is folded into the fields so an error keeps its stack without
// turning one log line into fifty grid rows.
var (
	timeKeys  = []string{"ts", "time", "timestamp"}
	levelKeys = []string{"level", "lvl", "severity"}
	msgKeys   = []string{"msg", "message"}
	skipKeys  = map[string]struct{}{
		"ts": {}, "time": {}, "timestamp": {},
		"level": {}, "lvl": {}, "severity": {},
		"msg": {}, "message": {},
	}
)

// ParseLine reads one line of the server's console output. The API writes JSON
// records (pkg/logger.NewJson); a line that is not JSON — a panic, a `log.Fatal`
// prefix, output from something the server started — is kept as it is.
func ParseLine(raw string) LogLine {
	text := stripANSI(strings.TrimSuffix(raw, "\r"))

	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		if line, ok := parseJSONLine(text); ok {
			return line
		}
	}

	return parseTextLine(text)
}

// Format lays a line out for a monospaced grid: time, level, message, then the
// fields. The level column is fixed width so the messages line up; a line that
// is not a log record is indented instead, which keeps a panic stack readable.
func Format(line LogLine) string {
	if line.Level == LevelOther && line.At.IsZero() {
		if text := oneLine(line.Raw); text != "" {
			return "  " + text
		}

		return ""
	}

	var text strings.Builder

	text.WriteString(pad(line.Time, 8))
	text.WriteString("  ")
	text.WriteString(pad(line.Level.String(), 5))

	if line.Message != "" {
		text.WriteString("  ")
		text.WriteString(line.Message)
	}

	for _, field := range line.Fields {
		text.WriteString("  ")
		text.WriteString(field.Key)
		text.WriteString("=")
		text.WriteString(field.Value)
	}

	return strings.TrimRight(text.String(), " ")
}

// parseJSONLine reads a structured record. A JSON object that says neither a
// message nor a level is not a log record — it is reported as text — so an
// unrelated JSON line is never silently reduced to its fields.
func parseJSONLine(text string) (LogLine, bool) {
	var values map[string]json.RawMessage

	if err := json.Unmarshal([]byte(text), &values); err != nil || len(values) == 0 {
		return LogLine{}, false
	}

	message, hasMessage := jsonString(values, msgKeys...)
	levelName, hasLevel := jsonString(values, levelKeys...)
	if !hasMessage && !hasLevel {
		return LogLine{}, false
	}

	line := LogLine{Raw: text, Message: oneLine(message)}

	if level, ok := parseLevel(levelName); ok {
		line.Level = level
	}

	if stamp, ok := jsonString(values, timeKeys...); ok {
		if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
			line.At = at
			line.Time = at.Local().Format("15:04:05")
		}
	}

	line.Fields = sortedFields(values)

	return line, true
}

// parseTextLine reads a plain line: a level word, then a message, then an
// optional tab-separated JSON object (the zap console encoder's shape).
func parseTextLine(text string) LogLine {
	line := LogLine{Raw: text}

	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return line
	}

	head, rest := splitHead(trimmed)

	level, ok := parseLevel(head)
	if !ok {
		return line
	}

	line.Level = level

	rest = strings.TrimSpace(rest)
	if message, fields, found := strings.Cut(rest, "\t{"); found {
		line.Message = oneLine(strings.TrimSpace(message))
		if parsed, ok := jsonFields("{" + fields); ok {
			line.Fields = parsed
		}

		return line
	}

	line.Message = oneLine(rest)

	return line
}

// splitHead cuts the first word of a line, stopping at a tab or a space.
func splitHead(text string) (string, string) {
	if index := strings.IndexAny(text, " \t"); index >= 0 {
		return text[:index], text[index+1:]
	}

	return text, ""
}

// parseLevel maps the words a logger uses onto the five levels the window
// knows. Anything else is not a level, and the line is not a record.
func parseLevel(word string) (Level, bool) {
	switch strings.ToUpper(strings.TrimSpace(word)) {
	case "DEBUG":
		return LevelDebug, true
	case "INFO", "NOTICE":
		return LevelInfo, true
	case "WARN", "WARNING":
		return LevelWarn, true
	case "ERROR", "DPANIC", "PANIC", "FATAL", "CRITICAL":
		return LevelError, true
	}

	return LevelOther, false
}

// sortedFields turns the record's leftover keys into fields, sorted so the
// same line always renders the same way.
func sortedFields(values map[string]json.RawMessage) []LogField {
	keys := make([]string, 0, len(values))

	for key := range values {
		if _, skip := skipKeys[key]; skip {
			continue
		}

		keys = append(keys, key)
	}

	sort.Strings(keys)

	fields := make([]LogField, 0, len(keys))

	for _, key := range keys {
		fields = append(fields, LogField{Key: key, Value: oneLine(jsonValue(values[key]))})
	}

	return fields
}

// jsonFields parses a JSON object into fields, for the console-encoded tail.
func jsonFields(text string) ([]LogField, bool) {
	var values map[string]json.RawMessage

	if err := json.Unmarshal([]byte(text), &values); err != nil {
		return nil, false
	}

	return sortedFields(values), true
}

// jsonString returns the first key that holds a string.
func jsonString(values map[string]json.RawMessage, keys ...string) (string, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}

		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			return text, true
		}
	}

	return "", false
}

// jsonValue is a value the way the server wrote it: a string without its
// quotes, anything else as its JSON text.
func jsonValue(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}

	return string(raw)
}

// oneLine flattens anything that would otherwise add a row to the grid. The
// log file keeps the original; the window keeps one row per line of output,
// which is what makes a row's style belong to a line.
func oneLine(text string) string {
	text = strings.ReplaceAll(text, "\r\n", " ")
	text = strings.ReplaceAll(text, "\n", " | ")
	text = strings.ReplaceAll(text, "\r", " ")

	return strings.TrimSpace(text)
}

// pad fills a value out to width, on the right, in spaces.
func pad(value string, width int) string {
	if len(value) >= width {
		return value
	}

	return value + strings.Repeat(" ", width-len(value))
}

// stripANSI removes terminal colour sequences. The API's console encoder is
// JSON, but a colour level encoder or a tool the server starts writes escapes,
// and an escape sequence in a grid is a row of boxes.
func stripANSI(text string) string {
	if !strings.ContainsRune(text, '\x1b') {
		return text
	}

	var out strings.Builder
	out.Grow(len(text))

	for i := 0; i < len(text); i++ {
		if text[i] != '\x1b' {
			out.WriteByte(text[i])

			continue
		}

		i++

		if i < len(text) && text[i] == '[' {
			// CSI: parameters until a final byte in @..~
			for i++; i < len(text); i++ {
				if text[i] >= 0x40 && text[i] <= 0x7e {
					break
				}
			}
		}
	}

	return out.String()
}
