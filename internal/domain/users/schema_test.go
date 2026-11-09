package users

import (
	"strings"
	"testing"

	"github.com/fiwon123/eznit/internal/domain/schematest"
)

// The migration is the schema, and the code that reads it has to agree with it.
// A read query that returns a column the struct cannot place is a runtime
// failure: the driver refuses to scan the row, the repository reports "not
// found", and the feature is broken on a table nobody looks at until a user
// does. That is how GET /v1/users/{id} answered 404 for every caller while its
// query named one column more than it scanned.
//
// The parser and the comparisons live in internal/domain/schematest, which the
// files, sessions and templates domains check with too.
const migrationsDir = "../../../db/migrations"

// readQuery is the shape of a read: SELECT <columns> FROM <table>.
func readQuery(columns, table string) string {
	return "SELECT " + columns + " FROM " + table
}

// The read query names the same columns the struct holds: every column the
// query returns has a place to go, and every place has a column.
func TestTheReadQueryMatchesTheStruct(t *testing.T) {
	schematest.SameColumns(t, "an account", readQuery(userColumns, "users"), User{})
}

// A read query that names `*` returns every column of the table, including the
// ones the struct does not hold — and the driver refuses to scan a row it
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

// Every column the read query names exists in the schema: a query that names a
// column the migrations do not create fails at runtime.
func TestTheReadQueryNamesColumnsThatExist(t *testing.T) {
	schema := schematest.Columns(t, migrationsDir, "users")

	schematest.ExistInTable(t, "an account", schema, schematest.QueryColumns(t, readQuery(userColumns, "users")), nil)
}

// The flag is written where the bootstrap puts it and nowhere else. An insert
// that forgot it would leave every instance without an admin; an update that
// named it would be a way to grant it over HTTP, which no route is meant to do.
func TestOnlyTheAccountCreationWritesTheAdminFlag(t *testing.T) {
	if !strings.Contains(createUserQuery, "is_admin") {
		t.Errorf("the signup insert does not write is_admin: %s", createUserQuery)
	}

	if strings.Contains(updateUserQuery, "is_admin") {
		t.Errorf("the account update names is_admin: %s", updateUserQuery)
	}
}
