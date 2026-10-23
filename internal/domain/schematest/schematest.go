// Package schematest holds the checks the domain tests use to keep a read
// query and the migration that defines its table in agreement.
//
// A read query that returns a column the struct cannot place is a runtime
// failure: sqlx refuses to scan the row, the repository answers "not found",
// and the feature is broken on a path nobody exercises until a user does. A
// query that reads `*` is exactly that trap, waiting for the next migration
// (it broke every read of a published template in #29, and the unit tests
// with a fake repository could not see it).
//
// The checks live here rather than in each package's _test.go because they are
// the same check in every domain, and three copies of a parser is three places
// for it to be wrong. The functions that make a judgement return it as a
// value, so they can be tested without a stand-in testing.T.
package schematest

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TableColumns returns the columns of a table according to the migrations in
// dir: the CREATE TABLE that introduces it plus every ALTER TABLE ... ADD
// COLUMN that follows, across all the .up.sql files, in filename order.
func TableColumns(dir, table string) ([]string, error) {
	migrations, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return nil, fmt.Errorf("list the migrations in %s: %w", dir, err)
	}
	if len(migrations) == 0 {
		return nil, fmt.Errorf("no .up.sql migrations in %s", dir)
	}
	sort.Strings(migrations)

	columns := map[string]bool{}
	found := false

	for _, migration := range migrations {
		content, err := os.ReadFile(migration)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", migration, err)
		}

		body := stripComments(string(content))

		for _, match := range createTablePattern.FindAllStringSubmatch(body, -1) {
			if !strings.EqualFold(match[1], table) {
				continue
			}
			found = true

			for _, part := range splitTopLevel(match[2]) {
				if name := columnName(part); name != "" {
					columns[name] = true
				}
			}
		}

		// One ALTER TABLE can add several columns in one statement, so the
		// statement is matched as a whole and the ADD COLUMNs inside it are
		// read one by one.
		for _, match := range alterTablePattern.FindAllStringSubmatch(body, -1) {
			if !strings.EqualFold(match[1], table) {
				continue
			}

			for _, add := range addColumnPattern.FindAllStringSubmatch(match[2], -1) {
				found = true
				columns[add[1]] = true
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("no migration creates the table %q", table)
	}

	return sortedKeys(columns), nil
}

// SelectColumns returns the column names in a SELECT list, sorted: the part
// after SELECT and before FROM, with table prefixes and aliases removed.
//
// It expects the list to be plain column references — `t.id`, `id AS file_id`
// — which is what a read that fills a struct uses.
func SelectColumns(query string) ([]string, error) {
	selectAt := strings.Index(strings.ToUpper(query), "SELECT ")
	if selectAt == -1 {
		return nil, fmt.Errorf("the query has no SELECT: %q", query)
	}

	list := query[selectAt+len("SELECT "):]

	// The first FROM at the top level ends the list. The search has to ignore
	// the parentheses: a column can be a subquery with a FROM of its own —
	// `(SELECT count(*) FROM template_files) AS file_count`.
	if fromAt := topLevelIndex(list, " FROM "); fromAt != -1 {
		list = list[:fromAt]
	}

	var columns []string

	for _, part := range splitTopLevel(list) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// The alias: `u.email AS owner_email`, and the alias of a subquery.
		if index := topLevelIndex(part, " AS "); index != -1 {
			part = part[index+len(" AS "):]
		}

		// The table prefix: `t.id`. A subquery has no prefix left after the
		// alias, so this only fires on a plain column.
		if index := strings.LastIndex(part, "."); index != -1 && topLevelIndex(part, "(") == -1 {
			part = part[index+1:]
		}

		columns = append(columns, strings.TrimSpace(part))
	}

	sort.Strings(columns)

	return columns, nil
}

// StructColumns returns the columns a struct can be scanned into: its `db`
// tags, sorted.
func StructColumns(model any) []string {
	columns := map[string]bool{}

	modelType := reflect.TypeOf(model)
	for modelType.Kind() == reflect.Pointer {
		modelType = modelType.Elem()
	}

	for i := 0; i < modelType.NumField(); i++ {
		tag := modelType.Field(i).Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		columns[tag] = true
	}

	return sortedKeys(columns)
}

// Difference reports how a query's column list and a struct's db tags differ.
// It returns "" when they are the same set.
func Difference(query string, model any) string {
	columns, err := SelectColumns(query)
	if err != nil {
		return err.Error()
	}

	got, want := strings.Join(columns, ", "), strings.Join(StructColumns(model), ", ")
	if got != want {
		return fmt.Sprintf("the query names [%s] and the struct holds [%s]", got, want)
	}

	return ""
}

// Missing returns the columns the schema does not have, ignoring the ones a
// join or an aggregate legitimately adds.
func Missing(schema, columns, ignore []string) []string {
	var missing []string

	for _, column := range columns {
		if contains(ignore, column) || contains(schema, column) {
			continue
		}
		missing = append(missing, column)
	}

	return missing
}

// SelectStar returns the first `SELECT *` in a repository source file, or ""
// when every read names its columns. The source is read as text on purpose: a
// query can be assembled at run time, and a check that ran the queries would
// miss the one that never runs in a test.
func SelectStar(path string) (string, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	return selectStarPattern.FindString(string(source)), nil
}

// Columns is TableColumns for a test: it fails the test instead of returning
// an error.
func Columns(t testing.TB, dir, table string) []string {
	t.Helper()

	columns, err := TableColumns(dir, table)
	if err != nil {
		t.Fatalf("%v", err)
	}

	return columns
}

// QueryColumns is SelectColumns for a test: it fails the test instead of
// returning an error.
func QueryColumns(t testing.TB, query string) []string {
	t.Helper()

	columns, err := SelectColumns(query)
	if err != nil {
		t.Fatalf("%v", err)
	}

	return columns
}

// SameColumns asserts that a query names exactly the columns a struct holds.
func SameColumns(t testing.TB, name, query string, model any) {
	t.Helper()

	if difference := Difference(query, model); difference != "" {
		t.Errorf("%s: %s", name, difference)
	}
}

// ExistInTable asserts that every column exists in the table's schema,
// ignoring the ones in ignore (a column from a join, a count).
func ExistInTable(t testing.TB, name string, schema, columns, ignore []string) {
	t.Helper()

	if missing := Missing(schema, columns, ignore); len(missing) > 0 {
		t.Errorf("%s: the query names %v, which the table does not have: %v", name, missing, schema)
	}
}

var (
	// createTablePattern finds a CREATE TABLE block in a migration.
	createTablePattern = regexp.MustCompile(`(?is)CREATE TABLE\s+(\w+)\s*\((.*?)\n\);`)

	// alterTablePattern finds a whole ALTER TABLE statement: the body can add
	// several columns, and it can wrap over several lines.
	alterTablePattern = regexp.MustCompile(`(?is)ALTER TABLE\s+(\w+)\s+(.*?);`)

	// addColumnPattern finds one added column inside an ALTER TABLE body.
	addColumnPattern = regexp.MustCompile(`(?is)ADD COLUMN\s+(?:IF NOT EXISTS\s+)?(\w+)`)

	// selectStarPattern finds a query that reads every column.
	selectStarPattern = regexp.MustCompile(`(?i)SELECT\s+\*`)
)

// columnName returns the name of a column definition and "" for a table
// constraint, which is not a column.
func columnName(definition string) string {
	definition = strings.TrimSpace(definition)
	if definition == "" {
		return ""
	}

	upper := strings.ToUpper(definition)
	for _, prefix := range []string{"PRIMARY", "FOREIGN", "UNIQUE", "CHECK", "CONSTRAINT"} {
		if strings.HasPrefix(upper, prefix) {
			return ""
		}
	}

	return strings.Fields(definition)[0]
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

// topLevelIndex returns the index of needle in s when it is not inside
// parentheses, or -1. The comparison ignores case, like SQL.
func topLevelIndex(s, needle string) int {
	upper, upperNeedle := strings.ToUpper(s), strings.ToUpper(needle)

	depth := 0
	for i := 0; i+len(upperNeedle) <= len(upper); i++ {
		switch upper[i] {
		case '(':
			depth++
		case ')':
			depth--
		}

		if depth == 0 && strings.HasPrefix(upper[i:], upperNeedle) {
			return i
		}
	}

	return -1
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

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
