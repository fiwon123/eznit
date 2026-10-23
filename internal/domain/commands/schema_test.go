package commands

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
// and the comparisons live in internal/domain/schematest, which the files,
// sessions and templates domains check with too.
const migrationsDir = "../../../db/migrations"

// The read queries name the same columns the struct can hold.
func TestTheReadQueriesMatchTheStructs(t *testing.T) {
	tests := []struct {
		name  string
		query string
		model any
	}{
		{"a command", "SELECT " + commandColumns + " FROM commands", Command{}},
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

// Every column the read queries name exists in the schema: a query that names
// a column the migration does not create fails at runtime.
func TestTheReadQueriesNameColumnsThatExist(t *testing.T) {
	schema := schematest.Columns(t, migrationsDir, "commands")

	schematest.ExistInTable(t, "a command", schema,
		schematest.QueryColumns(t, "SELECT "+commandColumns+" FROM commands"), nil)
}
