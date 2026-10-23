package schematest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The parser decides whether every domain's schema test passes, so it is
// tested here on migrations this test wrote itself. A parser that returned
// everything would make those tests pass vacuously.
func TestTableColumns(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "000001_create_things.up.sql"), `
-- A comment that mentions a column the table does not have: tricked_at.
CREATE TABLE things (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    owner_id    UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    kind        TEXT NOT NULL DEFAULT 'generic'
                CHECK (kind IN ('generic', 'other')),
    FOREIGN KEY (owner_id) REFERENCES owners (id),
    UNIQUE (name)
);
`)

	writeFile(t, filepath.Join(dir, "000002_add_things_size.up.sql"), `
ALTER TABLE things
ADD COLUMN size INTEGER NOT NULL DEFAULT 0;
`)

	writeFile(t, filepath.Join(dir, "000003_add_things_flag.up.sql"), `
ALTER TABLE other_things ADD COLUMN ignored BOOLEAN;
ALTER TABLE things ADD COLUMN IF NOT EXISTS archived BOOLEAN NOT NULL DEFAULT FALSE;
`)

	// A down migration must not be read.
	writeFile(t, filepath.Join(dir, "000002_add_things_size.down.sql"), `
ALTER TABLE things DROP COLUMN size;
`)

	columns, err := TableColumns(dir, "things")
	if err != nil {
		t.Fatalf("TableColumns() error = %v", err)
	}

	got, want := strings.Join(columns, ","), "archived,created_at,id,kind,name,owner_id,size"
	if got != want {
		t.Errorf("TableColumns() = %q, want %q", got, want)
	}
}

func TestTableColumnsRefusesAMissingTable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "000001_create_things.up.sql"), "CREATE TABLE things (id UUID);")

	if _, err := TableColumns(dir, "other"); err == nil {
		t.Error("TableColumns() returned no error for a table no migration creates")
	}

	if _, err := TableColumns(t.TempDir(), "things"); err == nil {
		t.Error("TableColumns() returned no error for a directory with no migrations")
	}
}

func TestSelectColumns(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{
			"a plain list",
			`SELECT id, name, ext FROM files WHERE id=$1`,
			"ext,id,name",
		},
		{
			"table prefixes and aliases",
			`SELECT f.id, f.name AS file_name, u.email AS owner_email FROM files f JOIN users u ON u.id = f.user_id`,
			"file_name,id,owner_email",
		},
		{
			"a list wrapped over lines",
			`SELECT id, user_id,
			        name, ext
			   FROM files`,
			"ext,id,name,user_id",
		},
		{
			"a subquery column",
			`SELECT t.id, (SELECT count(*) FROM template_files f WHERE f.template_id = t.id) AS file_count, u.email AS owner_email FROM templates t
			   JOIN users u ON u.id = t.user_id`,
			"file_count,id,owner_email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns, err := SelectColumns(tt.query)
			if err != nil {
				t.Fatalf("SelectColumns() error = %v", err)
			}

			if got := strings.Join(columns, ","); got != tt.want {
				t.Errorf("SelectColumns() = %q, want %q", got, tt.want)
			}
		})
	}

	if _, err := SelectColumns("UPDATE files SET name=$1"); err == nil {
		t.Error("SelectColumns() returned no error for a query with no SELECT")
	}
}

func TestStructColumns(t *testing.T) {
	type model struct {
		ID      string `db:"id"`
		Name    string `db:"name"`
		Ignored string
		Skip    string `db:"-"`
	}

	if got := strings.Join(StructColumns(model{}), ","); got != "id,name" {
		t.Errorf("StructColumns() = %q, want %q", got, "id,name")
	}

	if got := strings.Join(StructColumns(&model{}), ","); got != "id,name" {
		t.Errorf("StructColumns(&model{}) = %q, want %q", got, "id,name")
	}
}

// A helper that cannot report a difference is the bug this package exists to
// prevent, so the judgement is checked as a value.
func TestDifference(t *testing.T) {
	type model struct {
		ID   string `db:"id"`
		Name string `db:"name"`
	}

	if difference := Difference("SELECT id, name FROM models", model{}); difference != "" {
		t.Errorf("Difference() = %q for a query that matches the struct", difference)
	}

	if difference := Difference("SELECT id, ext FROM models", model{}); difference == "" {
		t.Error("Difference() reported nothing for a query that names a column the struct does not hold")
	}

	if difference := Difference("SELECT * FROM models", model{}); difference == "" {
		t.Error("Difference() reported nothing for a SELECT *")
	}
}

func TestMissing(t *testing.T) {
	schema := []string{"id", "name"}

	if missing := Missing(schema, []string{"id", "name"}, nil); len(missing) != 0 {
		t.Errorf("Missing() = %v, want none", missing)
	}

	if missing := Missing(schema, []string{"id", "ext"}, nil); len(missing) != 1 || missing[0] != "ext" {
		t.Errorf("Missing() = %v, want [ext]", missing)
	}

	// A column a join or an aggregate adds is not missing from the table.
	if missing := Missing(schema, []string{"id", "owner_email"}, []string{"owner_email"}); len(missing) != 0 {
		t.Errorf("Missing() = %v, want none when the column is ignored", missing)
	}
}

func TestSelectStar(t *testing.T) {
	dir := t.TempDir()

	clean := filepath.Join(dir, "clean.go")
	writeFile(t, clean, `query := "SELECT id FROM files"`)

	found, err := SelectStar(clean)
	if err != nil {
		t.Fatalf("SelectStar() error = %v", err)
	}
	if found != "" {
		t.Errorf("SelectStar() = %q, want empty", found)
	}

	dirty := filepath.Join(dir, "dirty.go")
	writeFile(t, dirty, "query := `SELECT * FROM files`")

	found, err = SelectStar(dirty)
	if err != nil {
		t.Fatalf("SelectStar() error = %v", err)
	}
	if !strings.HasPrefix(strings.ToUpper(found), "SELECT *") {
		t.Errorf("SelectStar() = %q, want a SELECT *", found)
	}

	if _, err := SelectStar(filepath.Join(dir, "missing.go")); err == nil {
		t.Error("SelectStar() returned no error for a file that is not there")
	}
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()

	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
