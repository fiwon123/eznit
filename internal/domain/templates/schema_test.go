package templates

import (
	"testing"

	"github.com/fiwon123/eznit/internal/domain/schematest"
)

// The migration is the schema, and the code that reads it has to agree with
// it. A read query that returns a column the struct cannot place is a runtime
// failure: sqlx refuses to scan the row, the repository reports "not found",
// and the feature is broken on a table nobody looks at until a user does.
//
// So the agreement is checked here, against the migration files. The parser
// and the comparisons live in internal/domain/schematest, which the files and
// the sessions domains check with too.
const migrationsDir = "../../../db/migrations"

// readQuery is the shape of a read: SELECT <columns> FROM <table>.
func readQuery(columns, table string) string {
	return "SELECT " + columns + " FROM " + table
}

// The read queries name the same columns the structs can hold: every column
// a query returns has a place to go, and every place has a column.
func TestTheReadQueriesMatchTheStructs(t *testing.T) {
	tests := []struct {
		name  string
		query string
		model any
	}{
		{"a template", readQuery(templateColumns, "templates"), Template{}},
		{"a file of a template", readQuery(templateFileColumns, "template_files"), TemplateFile{}},
		{"a variable of a template", readQuery(templateVariableColumns, "template_variables"), TemplateVariable{}},
		{"a command a template pulls in", readQuery(templateCommandColumns, "template_commands"), TemplateCommand{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schematest.SameColumns(t, tt.name, tt.query, tt.model)
		})
	}
}

// A read query that names `*` returns every column of the table, including
// the ones the struct does not hold — and sqlx refuses to scan a row it
// cannot place. The columns are named, one by one.
func TestTheRepositoryNeverReadsEveryColumn(t *testing.T) {
	found, err := schematest.SelectStar("repository_sql.go")
	if err != nil {
		t.Fatalf("failed to read the repository: %v", err)
	}

	if found != "" {
		t.Errorf("expected every query to name its columns, found %q", found)
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
		{"a command reference", "template_commands", templateCommandRefColumns, nil},
		{"the command a reference points at", "commands", templateCommandCommandColumns, []string{"owner_id"}},
		{
			"a template",
			"templates",
			templateColumns,
			// The owner's email comes from the users table and the file
			// count is counted by the database.
			[]string{"owner_email", "file_count"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := schematest.Columns(t, migrationsDir, tt.table)

			schematest.ExistInTable(t, tt.name, schema, schematest.QueryColumns(t, readQuery(tt.query, tt.table)), tt.ignore)
		})
	}
}
