package templates

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The migration is the schema, and the code that reads it has to agree with
// it.
//
// A read query that returns a column the struct cannot place is a runtime
// failure: sqlx refuses to scan the row, the repository reports "not found",
// and the feature is broken on a table nobody looks at until a user does. A
// query that reads `*` is exactly that trap, waiting for the next migration.
//
// So the agreement is checked here, against the migration file itself.

const migrationPath = "../../../db/migrations/000009_create_templates.up.sql"

// createTablePattern finds a CREATE TABLE block in the migration.
var createTablePattern = regexp.MustCompile(`(?is)CREATE TABLE\s+(\w+)\s*\((.*?)\n\);`)

// selectStarPattern finds a query that reads every column.
var selectStarPattern = regexp.MustCompile(`(?i)SELECT\s+\*`)

// migrationTableColumns returns the columns of a table, read from the
// migration file.
func migrationTableColumns(t *testing.T, table string) []string {
	t.Helper()

	content, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("failed to read the migration: %v", err)
	}

	for _, match := range createTablePattern.FindAllStringSubmatch(stripComments(string(content)), -1) {
		if match[1] != table {
			continue
		}

		var columns []string

		for _, part := range splitTopLevel(match[2]) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			// A constraint is not a column.
			upper := strings.ToUpper(part)
			if strings.HasPrefix(upper, "PRIMARY") ||
				strings.HasPrefix(upper, "FOREIGN") ||
				strings.HasPrefix(upper, "UNIQUE") ||
				strings.HasPrefix(upper, "CHECK") ||
				strings.HasPrefix(upper, "CONSTRAINT") {
				continue
			}

			columns = append(columns, strings.Fields(part)[0])
		}

		return columns
	}

	t.Fatalf("the migration has no table %q", table)

	return nil
}

// stripComments removes the line comments of a SQL file: a comment is not a
// column, and the parser would read one as several.
func stripComments(sql string) string {
	var b strings.Builder

	for _, line := range strings.Split(sql, "\n") {
		if index := strings.Index(line, "--"); index != -1 {
			line = line[:index]
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

// splitTopLevel splits a block of SQL on the commas that are not inside
// parentheses: a CHECK constraint holds a list of values.
func splitTopLevel(block string) []string {
	var parts []string

	depth := 0
	current := strings.Builder{}

	for _, r := range block {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, current.String())
				current.Reset()
				continue
			}
		}

		current.WriteRune(r)
	}

	if strings.TrimSpace(current.String()) != "" {
		parts = append(parts, current.String())
	}

	return parts
}

// dbTagsOf returns the columns a struct can be scanned into.
func dbTagsOf(value any) []string {
	tags := map[string]bool{}

	valueType := reflect.TypeOf(value)
	for i := 0; i < valueType.NumField(); i++ {
		field := valueType.Field(i)
		tag := field.Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		tags[tag] = true
	}

	columns := make([]string, 0, len(tags))
	for tag := range tags {
		columns = append(columns, tag)
	}

	sort.Strings(columns)

	return columns
}

// queryColumns returns the columns a query names, without the table prefix
// and without the alias.
func queryColumns(query string) []string {
	var columns []string

	for _, part := range strings.Split(query, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// The alias: `u.email AS owner_email`.
		if index := strings.Index(strings.ToUpper(part), " AS "); index != -1 {
			part = part[index+4:]
		}

		// The table prefix: `t.id`.
		if index := strings.LastIndex(part, "."); index != -1 {
			part = part[index+1:]
		}

		columns = append(columns, strings.TrimSpace(part))
	}

	sort.Strings(columns)

	return columns
}

// The read queries name the same columns the structs can hold: every column
// a query returns has a place to go, and every place has a column.
func TestTheReadQueriesMatchTheStructs(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		structv any
	}{
		{"a template", templateColumns, Template{}},
		{"a file of a template", templateFileColumns, TemplateFile{}},
		{"a variable of a template", templateVariableColumns, TemplateVariable{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := queryColumns(tt.query)
			structColumns := dbTagsOf(tt.structv)

			if len(query) != len(structColumns) {
				t.Fatalf("the query names %v and the struct holds %v", query, structColumns)
			}

			for i := range query {
				if query[i] != structColumns[i] {
					t.Errorf("the query names %q and the struct holds %q", query[i], structColumns[i])
				}
			}
		})
	}
}

// A read query that names `*` returns every column of the table, including
// the ones the struct does not hold — and sqlx refuses to scan a row it
// cannot place. The columns are named, one by one.
func TestTheRepositoryNeverReadsEveryColumn(t *testing.T) {
	source, err := os.ReadFile("repository_sql.go")
	if err != nil {
		t.Fatalf("failed to read the repository: %v", err)
	}

	match := selectStarPattern.FindString(string(source))
	if match != "" {
		t.Errorf("expected every query to name its columns, found %q", match)
	}
}

// Every column the read queries name exists in the schema: a query that
// names a column the migration does not create fails at runtime.
func TestTheReadQueriesNameColumnsThatExist(t *testing.T) {
	tests := []struct {
		name   string
		table  string
		query  string
		ignore []string
	}{
		{"a file of a template", "template_files", templateFileColumns, nil},
		{"a variable of a template", "template_variables", templateVariableColumns, nil},
		{
			"a template",
			"templates",
			templateColumns,
			// The owner's email comes from the users table, and the
			// columns of a template are prefixed with the table name.
			[]string{"owner_email"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := migrationTableColumns(t, tt.table)

			for _, column := range queryColumns(tt.query) {
				if contains(tt.ignore, column) {
					continue
				}

				if !contains(schema, column) {
					t.Errorf("the query names %q, which the table %q does not have: %v", column, tt.table, schema)
				}
			}
		})
	}
}

// contains reports whether a list holds a value.
func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}

	return false
}
