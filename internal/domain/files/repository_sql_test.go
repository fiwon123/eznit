package files

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// recordingDriver records the queries the repository sends to the database.
// The list is bounded in SQL, and a test that never reaches the database
// cannot tell a bounded query from an unbounded one — so this one does.
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

func (d *recordingDriver) allQueries() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return append([]string(nil), d.queries...)
}

func (d *recordingDriver) lastQuery() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.queries) == 0 {
		return ""
	}

	return d.queries[len(d.queries)-1]
}

type recordingConn struct {
	driver *recordingDriver
}

func (c *recordingConn) Prepare(query string) (driver.Stmt, error) {
	c.driver.record(query)
	return &recordingStmt{}, nil
}

func (c *recordingConn) Close() error { return nil }

func (c *recordingConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported by the recording driver")
}

type recordingStmt struct{}

func (s *recordingStmt) Close() error { return nil }

func (s *recordingStmt) NumInput() int { return -1 }

func (s *recordingStmt) Exec(args []driver.Value) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}

func (s *recordingStmt) Query(args []driver.Value) (driver.Rows, error) {
	return &recordingRows{}, nil
}

type recordingRows struct{}

func (r *recordingRows) Columns() []string { return []string{"id"} }

func (r *recordingRows) Close() error { return nil }

func (r *recordingRows) Next(dest []driver.Value) error { return io.EOF }

// recordingRepository returns a repository whose database records every
// query instead of running it: the SQL is what these tests are about.
func recordingRepository(t *testing.T) (*sqlRepository, *recordingDriver) {
	t.Helper()

	recorder := &recordingDriver{}

	// A driver name is global to the process, so each test registers its own.
	name := "recording-driver-" + strings.ReplaceAll(t.Name(), "/", "-")
	sql.Register(name, recorder)

	db, err := sqlx.Open(name, "not-a-real-database")
	if err != nil {
		t.Fatalf("failed to open the recording database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	return NewRepository(db, log), recorder
}

// The query that reaches the database is bounded: the list is a catalogue
// of every upload, and an unbounded read is what made the route expensive.
func TestGetFilesSendsABoundedQuery(t *testing.T) {
	repo, recorder := recordingRepository(t)

	if _, ok := repo.GetFiles(100); !ok {
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

// An update moves the index row and must leave `files_history` alone: the
// rows of the versions before it are the record of what was written and
// where. Rewriting them (the old statement was
// `UPDATE files_history SET path=..., version=... WHERE file_id=...`) loses
// the previous version and points every row at the newest file (#65).
func TestUpdateFileDoesNotRewriteTheHistory(t *testing.T) {
	repo, recorder := recordingRepository(t)

	repo.UpdateFile(File{ID: uuid.New(), UserID: uuid.New(), Version: 2})

	queries := recorder.allQueries()
	if len(queries) == 0 {
		t.Fatal("expected a query to reach the database")
	}

	for _, query := range queries {
		if strings.Contains(strings.ToLower(query), "files_history") {
			t.Errorf("UpdateFile touched files_history: %q", query)
		}
	}

	if !strings.Contains(recorder.lastQuery(), "UPDATE files SET") {
		t.Errorf("expected the index row to be updated, sent %v", queries)
	}
}
