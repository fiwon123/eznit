package templates

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// recordingDriver records the queries the repository sends to the database.
// The rules that live in the SQL — the catalogue is bounded, the catalogue is
// filtered by owner, a delete is scoped by owner — cannot be seen by a test
// that never reaches the database, so this one does.
type recordingDriver struct {
	mu      sync.Mutex
	queries []string
}

func (d *recordingDriver) Open(name string) (driver.Conn, error) {
	return &recordingConn{driver: d}, nil
}

func (d *recordingDriver) record(query string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queries = append(d.queries, query)
}

func (d *recordingDriver) lastQuery() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.queries) == 0 {
		return ""
	}

	return d.queries[len(d.queries)-1]
}

// allQueries returns every query the driver saw.
func (d *recordingDriver) allQueries() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return append([]string{}, d.queries...)
}

type recordingConn struct {
	driver *recordingDriver
}

func (c *recordingConn) Prepare(query string) (driver.Stmt, error) {
	c.driver.record(query)
	return &recordingStmt{}, nil
}

func (c *recordingConn) Close() error { return nil }

// Begin returns a transaction that does nothing: the recording driver only
// cares about the queries that reach it.
func (c *recordingConn) Begin() (driver.Tx, error) {
	return &recordingTx{}, nil
}

type recordingTx struct{}

func (t *recordingTx) Commit() error   { return nil }
func (t *recordingTx) Rollback() error { return nil }

type recordingStmt struct{}

func (s *recordingStmt) Close() error  { return nil }
func (s *recordingStmt) NumInput() int { return -1 }

func (s *recordingStmt) Exec(args []driver.Value) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}

func (s *recordingStmt) Query(args []driver.Value) (driver.Rows, error) {
	return &recordingRows{}, nil
}

type recordingRows struct{}

func (r *recordingRows) Columns() []string { return []string{"id"} }
func (r *recordingRows) Close() error      { return nil }
func (r *recordingRows) Next(dest []driver.Value) error {
	return io.EOF
}

// driverCounter names every driver separately: sql.Register panics when the
// same name is registered twice, and every test in this package needs its own
// recorder.
var driverCounter atomic.Int64

// newRecordingRepository returns a repository whose queries are recorded, and
// the recorder holding them.
func newRecordingRepository(t *testing.T) (*sqlRepository, *recordingDriver) {
	t.Helper()

	recorder := &recordingDriver{}
	name := fmt.Sprintf("templates-recording-driver-%d", driverCounter.Add(1))
	sql.Register(name, recorder)

	db, err := sqlx.Open(name, "not-a-real-database")
	if err != nil {
		t.Fatalf("failed to open the recording database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	return NewRepository(db, log), recorder
}

// The catalogue is a list of templates: an unbounded read is what made the
// files route expensive, so the query has to carry a limit.
func TestListTemplatesSendsABoundedQuery(t *testing.T) {
	repo, recorder := newRecordingRepository(t)

	if _, ok := repo.ListTemplates(uuid.New(), "", 100); !ok {
		t.Fatal("expected the query to succeed")
	}

	query := recorder.lastQuery()
	if query == "" {
		t.Fatal("expected a query to reach the database")
	}

	upper := strings.ToUpper(query)
	if !strings.Contains(upper, "LIMIT") && !strings.Contains(upper, "FETCH FIRST") {
		t.Errorf("expected the query to be bounded, got %q", query)
	}
}

// The catalogue is filtered in the query, by the owner and by the visibility:
// a template that is not visible to the caller is never read in the first
// place.
func TestListTemplatesFiltersByOwnerAndVisibility(t *testing.T) {
	repo, recorder := newRecordingRepository(t)

	if _, ok := repo.ListTemplates(uuid.New(), KindCodex, 100); !ok {
		t.Fatal("expected the query to succeed")
	}

	query := strings.ToUpper(recorder.lastQuery())

	if !strings.Contains(query, "IS_PUBLIC") {
		t.Errorf("expected the query to filter by visibility, got %q", query)
	}

	if !strings.Contains(query, "USER_ID") {
		t.Errorf("expected the query to filter by owner, got %q", query)
	}

	if !strings.Contains(query, "KIND") {
		t.Errorf("expected the query to filter by kind, got %q", query)
	}
}

// A delete is scoped by the owner in the query itself: the service checks
// the ownership, and the statement does not depend on that check alone.
func TestDeleteTemplateIsScopedByOwner(t *testing.T) {
	repo, recorder := newRecordingRepository(t)

	repo.DeleteTemplate(uuid.New(), uuid.New())

	query := strings.ToUpper(recorder.lastQuery())
	if !strings.Contains(query, "USER_ID") {
		t.Errorf("expected the delete to be scoped by owner, got %q", query)
	}
}

// A template, its files and its variables are written in one transaction: the
// three statements belong to the same transaction, so a template without its
// files cannot be left behind.
func TestCreateTemplateWritesTheFilesAndTheVariables(t *testing.T) {
	repo, recorder := newRecordingRepository(t)

	templateID := uuid.New()

	ok := repo.CreateTemplate(
		Template{ID: templateID, Name: "mine", Slug: "mine", Kind: KindGeneric},
		[]TemplateFile{{ID: uuid.New(), TemplateID: templateID, Path: "AGENTS.md", Content: "x"}},
		[]TemplateVariable{{ID: uuid.New(), TemplateID: templateID, Key: "PROJECT_NAME"}},
		nil,
	)

	if !ok {
		t.Fatal("expected the template to be created")
	}

	queries := recorder.allQueries()
	if len(queries) != 3 {
		t.Fatalf("expected 3 statements, got %d: %v", len(queries), queries)
	}

	joined := strings.ToUpper(strings.Join(queries, " "))

	for _, table := range []string{"TEMPLATES", "TEMPLATE_FILES", "TEMPLATE_VARIABLES"} {
		if !strings.Contains(joined, table) {
			t.Errorf("expected a statement on %s, got %v", table, queries)
		}
	}

	// The variables are stored in `var_key`, the column that is not a
	// keyword in Postgres.
	if !strings.Contains(joined, "VAR_KEY") {
		t.Errorf("expected the variables to be written with var_key, got %v", queries)
	}
}

// failingBeginDriver is a driver whose transactions cannot be started.
type failingBeginDriver struct {
	recorder *recordingDriver
}

func (d *failingBeginDriver) Open(name string) (driver.Conn, error) {
	return &failingBeginConn{driver: d.recorder}, nil
}

type failingBeginConn struct {
	driver *recordingDriver
}

func (c *failingBeginConn) Prepare(query string) (driver.Stmt, error) {
	c.driver.record(query)
	return &recordingStmt{}, nil
}

func (c *failingBeginConn) Close() error { return nil }

func (c *failingBeginConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported by this driver")
}

// A write that cannot be started fails as a whole: a template whose files
// did not make it is not a template, so nothing is stored.
func TestCreateTemplateFailsWithoutATransaction(t *testing.T) {
	recorder := &recordingDriver{}
	name := fmt.Sprintf("templates-failing-driver-%d", driverCounter.Add(1))
	sql.Register(name, &failingBeginDriver{recorder: recorder})

	db, err := sqlx.Open(name, "not-a-real-database")
	if err != nil {
		t.Fatalf("failed to open the recording database: %v", err)
	}
	defer db.Close()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	repo := NewRepository(db, log)

	if repo.CreateTemplate(
		Template{ID: uuid.New(), Name: "mine", Slug: "mine", Kind: KindGeneric},
		[]TemplateFile{{ID: uuid.New(), Path: "AGENTS.md", Content: "x"}},
		nil,
		nil,
	) {
		t.Fatal("expected the write to fail without a transaction")
	}

	for _, query := range recorder.allQueries() {
		if strings.Contains(strings.ToUpper(query), "INSERT") {
			t.Errorf("expected nothing to be written, got %q", query)
		}
	}
}
