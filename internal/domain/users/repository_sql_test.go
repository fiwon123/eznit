package users

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

// scriptedDriver answers every statement the way the test asks: no rows, a
// failure, or a number of affected rows. The repository's job in #148 is to
// translate exactly those answers, so this is what pins the translation down
// without a database.
type scriptedDriver struct {
	mu       sync.Mutex
	next     error // what a row cursor answers; io.EOF means "no rows"
	affected int64 // what an Exec reports as affected rows
	execErr  error // when set, Exec fails
	queries  []string
}

func (d *scriptedDriver) Open(string) (driver.Conn, error) {
	return &scriptedConn{driver: d}, nil
}

func (d *scriptedDriver) record(query string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.queries = append(d.queries, query)
}

func (d *scriptedDriver) recorded() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return append([]string(nil), d.queries...)
}

type scriptedConn struct {
	driver *scriptedDriver
}

func (c *scriptedConn) Prepare(query string) (driver.Stmt, error) {
	c.driver.record(query)
	return &scriptedStmt{driver: c.driver}, nil
}

func (c *scriptedConn) Close() error { return nil }

func (c *scriptedConn) Begin() (driver.Tx, error) { return scriptedTx{}, nil }

type scriptedTx struct{}

func (scriptedTx) Commit() error   { return nil }
func (scriptedTx) Rollback() error { return nil }

type scriptedStmt struct {
	driver *scriptedDriver
}

func (s *scriptedStmt) Close() error  { return nil }
func (s *scriptedStmt) NumInput() int { return -1 }

func (s *scriptedStmt) Exec([]driver.Value) (driver.Result, error) {
	if s.driver.execErr != nil {
		return nil, s.driver.execErr
	}

	return driver.RowsAffected(s.driver.affected), nil
}

func (s *scriptedStmt) Query([]driver.Value) (driver.Rows, error) {
	return &scriptedRows{driver: s.driver}, nil
}

type scriptedRows struct {
	driver *scriptedDriver
}

func (r *scriptedRows) Columns() []string { return []string{"id"} }
func (r *scriptedRows) Close() error      { return nil }

func (r *scriptedRows) Next(destination []driver.Value) error {
	if r.driver.next == nil {
		return io.EOF
	}

	return r.driver.next
}

// scriptedRepository returns a repository over the given driver.
func scriptedRepository(t *testing.T, driver *scriptedDriver) *sqlRepository {
	t.Helper()

	// A driver name is global to the process, so each test registers its own.
	name := "scripted-driver-" + strings.ReplaceAll(t.Name(), "/", "-")
	sql.Register(name, driver)

	db, err := sqlx.Open(name, "not-a-real-database")
	if err != nil {
		t.Fatalf("failed to open the scripted database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	return NewRepository(db, log)
}

// A row that is not there is ErrNotFound; a query that failed is its own error.
// Collapsing the two is exactly the defect #148 fixes, and only the repository
// can tell them apart.
func TestTheReadsTellAMissingRowFromAFailedQuery(t *testing.T) {
	reads := []struct {
		name string
		read func(*sqlRepository) error
	}{
		{"a user by id", func(r *sqlRepository) error {
			_, err := r.GetUser(uuid.New())
			return err
		}},
		{"a user by email", func(r *sqlRepository) error {
			_, err := r.GetUserByEmail("nobody@example.com")
			return err
		}},
	}

	for _, read := range reads {
		t.Run(read.name+" that is not there", func(t *testing.T) {
			repo := scriptedRepository(t, &scriptedDriver{})

			err := read.read(repo)
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("expected ErrNotFound, got %v", err)
			}
		})

		t.Run(read.name+" whose query failed", func(t *testing.T) {
			failure := errors.New("the connection went away")
			repo := scriptedRepository(t, &scriptedDriver{next: failure})

			err := read.read(repo)
			if err == nil {
				t.Fatal("expected the failure to be reported")
			}

			if errors.Is(err, ErrNotFound) {
				t.Errorf("a failed query was reported as ErrNotFound: %v", err)
			}
		})
	}
}

// An update that changed no row is ErrNotFound — the account went between the
// read and the write — and an update that changed one is success.
func TestUpdateUserReportsWhetherItChangedARow(t *testing.T) {
	t.Run("no row", func(t *testing.T) {
		repo := scriptedRepository(t, &scriptedDriver{affected: 0})

		if err := repo.UpdateUser(User{ID: uuid.New()}); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("one row", func(t *testing.T) {
		repo := scriptedRepository(t, &scriptedDriver{affected: 1})

		if err := repo.UpdateUser(User{ID: uuid.New()}); err != nil {
			t.Errorf("expected the update to succeed, got %v", err)
		}
	})
}

// The account delete goes templates, then commands, then the account: the
// first two leave folders on disk, and the account row's cascade is what sweeps
// sessions and files. A transaction that ran them in another order would hit
// the RESTRICT reference or lose the ids it has to clean up.
func TestDeleteUserRunsItsStatementsInOrder(t *testing.T) {
	driver := &scriptedDriver{affected: 1}
	repo := scriptedRepository(t, driver)

	if _, err := repo.DeleteUser(User{ID: uuid.New()}); err != nil {
		t.Fatalf("expected the delete to succeed, got %v", err)
	}

	queries := strings.Join(driver.recorded(), "\n")

	templates := strings.Index(queries, "DELETE FROM templates")
	commands := strings.Index(queries, "DELETE FROM commands")
	account := strings.Index(queries, "DELETE FROM users")

	if templates == -1 || commands == -1 || account == -1 {
		t.Fatalf("expected the three deletes, got:\n%s", queries)
	}

	if !(templates < commands && commands < account) {
		t.Errorf("expected templates, then commands, then the account, got:\n%s", queries)
	}
}

// A delete that removed no row is ErrNotFound: another request deleted the
// account between the read and the transaction.
func TestDeleteUserReportsAMissingRowAsNotFound(t *testing.T) {
	repo := scriptedRepository(t, &scriptedDriver{affected: 0})

	if _, err := repo.DeleteUser(User{ID: uuid.New()}); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
