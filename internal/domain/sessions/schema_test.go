package sessions

import (
	"testing"

	"github.com/fiwon123/eznit/internal/domain/schematest"
)

const migrationsDir = "../../../db/migrations"

// The shape of every read of the sessions table: SELECT + sessionColumns.
const readSessionsQuery = "SELECT " + sessionColumns + " FROM sessions"

// A read returns exactly the columns the struct can scan: one that returns
// more is a runtime failure that the repository reports as "no session".
func TestTheSessionsReadMatchesTheStruct(t *testing.T) {
	schematest.SameColumns(t, "the sessions read", readSessionsQuery, Session{})
}

func TestTheSessionsRepositoryNeverReadsEveryColumn(t *testing.T) {
	found, err := schematest.SelectStar("repository_sql.go")
	if err != nil {
		t.Fatalf("failed to read the repository: %v", err)
	}

	if found != "" {
		t.Errorf("expected every query to name its columns, found %q", found)
	}
}

func TestTheSessionColumnsExistInTheSchema(t *testing.T) {
	schema := schematest.Columns(t, migrationsDir, "sessions")

	schematest.ExistInTable(t, "the Session struct", schema, schematest.StructColumns(Session{}), nil)
}
