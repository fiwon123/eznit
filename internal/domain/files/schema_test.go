package files

import (
	"testing"

	"github.com/fiwon123/eznit/internal/domain/schematest"
)

// The files table gains columns over time (content_type in 000006, version in
// 000008), and a read has to keep up with it. These checks keep the read, the
// struct and the migration in agreement, so the next migration cannot break a
// read that no test happens to call.
const migrationsDir = "../../../db/migrations"

// The shape of every read of the files table: SELECT + fileColumns.
const readFilesQuery = "SELECT " + fileColumns + " FROM files"

// A read returns exactly the columns the struct can scan: one that returns
// more is a runtime "not found", one that returns fewer leaves fields empty.
func TestTheFilesReadMatchesTheStruct(t *testing.T) {
	schematest.SameColumns(t, "the files read", readFilesQuery, File{})
}

// A read query that names `*` returns every column of the table, including
// the ones the struct does not hold — and sqlx refuses to scan a row it
// cannot place.
func TestTheFilesRepositoryNeverReadsEveryColumn(t *testing.T) {
	found, err := schematest.SelectStar("repository_sql.go")
	if err != nil {
		t.Fatalf("failed to read the repository: %v", err)
	}

	if found != "" {
		t.Errorf("expected every query to name its columns, found %q", found)
	}
}

// Every column the struct holds exists in the schema, as the migrations build
// it: CREATE TABLE files plus the columns added later.
func TestTheFileColumnsExistInTheSchema(t *testing.T) {
	schema := schematest.Columns(t, migrationsDir, "files")

	schematest.ExistInTable(t, "the File struct", schema, schematest.StructColumns(File{}), nil)
}
