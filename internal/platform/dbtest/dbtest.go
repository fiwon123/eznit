//go:build integration

// Package dbtest is the fixture the live-PostgreSQL integration suite runs on:
// one connection, the repository's migrations applied, a guard that refuses a
// database that is not clearly a test one, and a reset between tests.
//
// Everything here is behind the `integration` build tag, so `go test ./...`
// and `make check` never compile it and never need a database. Run it with
// `make test_integration` and a TEST_DATABASE_URL whose database name ends in
// `_test`.
package dbtest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/jmoiron/sqlx"

	// The driver the product uses, so a query that only works under a
	// different driver fails here too.
	_ "github.com/lib/pq"
)

// Env names the connection string the suite reads. It is deliberately not
// DB_*: those point at the product's database, and this suite drops and
// rewrites tables.
const Env = "TEST_DATABASE_URL"

// lockKey serialises the packages. `go test ./...` runs packages in parallel
// and every package shares one database, so without this two packages would
// truncate each other's rows. It is an arbitrary constant, stable across runs.
const lockKey = 0x657a6e6974 // "eznit", little-endian

var (
	migrateOnce sync.Once
	migrateErr  error
)

// Open connects to Env, refuses a database whose name does not end in `_test`,
// takes the cross-package advisory lock, and makes sure the migrations have
// run. The connection and the lock are released when the test ends.
func Open(t *testing.T) *sqlx.DB {
	t.Helper()

	dsn := os.Getenv(Env)
	if dsn == "" {
		t.Fatalf("%s is not set: run `make test_integration` against a scratch database whose name ends in _test", Env)
	}

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatalf("connect to %s: %v", Env, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	RequireTestDatabase(t, db)

	// The lock outlives a test that fails early; closing the connection is
	// what releases it, which the cleanup above does. The explicit unlock
	// keeps a long-lived connection from carrying it into the next test.
	if _, err := db.Exec("SELECT pg_advisory_lock($1)", int64(lockKey)); err != nil {
		t.Fatalf("take the advisory lock: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec("SELECT pg_advisory_unlock($1)", int64(lockKey)) })

	migrateOnce.Do(func() { migrateErr = MigrateUpNow(db) })
	if migrateErr != nil {
		t.Fatalf("apply the migrations: %v", migrateErr)
	}

	return db
}

// RequireTestDatabase aborts the test unless the connected database's name ends
// in `_test`. Everything below this line is destructive: a mistyped
// TEST_DATABASE_URL must fail the run, not wipe a real database.
func RequireTestDatabase(t *testing.T, db *sqlx.DB) {
	t.Helper()

	var name string
	if err := db.Get(&name, "SELECT current_database()"); err != nil {
		t.Fatalf("read current_database(): %v", err)
	}

	if !isTestDatabase(name) {
		t.Fatalf("refusing to run: database %q does not end in _test, and this suite drops and truncates tables", name)
	}
}

// isTestDatabase is the guard as a value, so it can be tested without a
// connection. The suffix is the whole rule: a name is either clearly a test
// database or it is not.
func isTestDatabase(name string) bool {
	return strings.HasSuffix(name, "_test")
}

// MigrateUpNow drops the public schema, recreates it, and applies every
// `db/migrations/*.up.sql` in filename order. Starting from an empty schema is
// what makes the run repeatable on a database that already has the tables.
//
// It speaks SQL rather than `golang-migrate`: the migrate tool is the product's
// container, not a Go dependency, and a test that pinned a second migration
// runner could disagree with the one that runs in production.
func MigrateUpNow(db *sqlx.DB) error {
	if _, err := db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		return fmt.Errorf("reset the public schema: %w", err)
	}
	return applyMigrations(db, ".up.sql", false)
}

// MigrateUp applies the up migrations to the current schema without dropping
// it. The migration test uses it after reversing them.
func MigrateUp(t *testing.T, db *sqlx.DB) {
	t.Helper()

	if err := applyMigrations(db, ".up.sql", false); err != nil {
		t.Fatalf("apply the up migrations: %v", err)
	}
}

// MigrateDown applies every `db/migrations/*.down.sql` in reverse filename
// order, which is the order the migrate tool reverses them in.
func MigrateDown(t *testing.T, db *sqlx.DB) {
	t.Helper()

	if err := applyMigrations(db, ".down.sql", true); err != nil {
		t.Fatalf("apply the down migrations: %v", err)
	}
}

// Reset empties every domain table and restarts the sequences, so a test
// starts from the same state regardless of what ran before it.
func Reset(t *testing.T, db *sqlx.DB) {
	t.Helper()

	const tables = `users, sessions, files, files_history, templates,
		template_files, template_variables, commands, template_commands`

	if _, err := db.Exec("TRUNCATE " + tables + " RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate the domain tables: %v", err)
	}
}

// Logger returns a logger that writes to the test's temporary directory
// instead of stdio. The repositories log through it, and a test run should not
// bury a real failure under their output.
func Logger(t *testing.T) *logger.Config {
	t.Helper()

	cfg, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("build the test logger: %v", err)
	}
	return cfg
}

// Count returns the number of rows in table matching a condition. The table
// name comes from the test, never from a request.
func Count(t *testing.T, db *sqlx.DB, table, condition string, args ...any) int {
	t.Helper()

	var count int
	if err := db.Get(&count, "SELECT count(*) FROM "+table+" WHERE "+condition, args...); err != nil {
		t.Fatalf("count %s where %s: %v", table, condition, err)
	}
	return count
}

// applyMigrations reads the migrations from db/migrations and executes them in
// filename order (reverse for the down files). lib/pq runs a multi-statement
// file through the simple protocol, so a file with several statements is
// executed as the migrate tool executes it.
func applyMigrations(db *sqlx.DB, suffix string, reverse bool) error {
	dir, err := migrationsDir()
	if err != nil {
		return err
	}

	files, err := filepath.Glob(filepath.Join(dir, "*"+suffix))
	if err != nil {
		return fmt.Errorf("list %s migrations: %w", suffix, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no %s migrations in %s", suffix, dir)
	}
	sort.Strings(files)
	if reverse {
		sort.Sort(sort.Reverse(sort.StringSlice(files)))
	}

	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read %s: %w", file, err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(file), err)
		}
	}

	return nil
}

// migrationsDir walks up from the working directory to the module root, which
// is where db/migrations lives. The test's working directory is its package
// directory, and that depth differs per package.
func migrationsDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("working directory: %w", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "db", "migrations"), nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find go.mod above the working directory")
		}
		dir = parent
	}
}
