//go:build integration

package users

import (
	"errors"
	"testing"

	"github.com/fiwon123/eznit/internal/platform/dbtest"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func newUsersRepo(t *testing.T) (*sqlRepository, *sqlx.DB) {
	t.Helper()
	db := dbtest.Open(t)
	dbtest.Reset(t, db)
	return NewRepository(db, dbtest.Logger(t)), db
}

// createUser writes an account and reads it back, which is also the assertion
// that the insert and the read name the same columns.
func createUser(t *testing.T, repo *sqlRepository, email string, admin bool) User {
	t.Helper()

	if err := repo.CreateUser(User{Email: email, Password: []byte("hash:" + email), IsAdmin: admin}); err != nil {
		t.Fatalf("CreateUser(%s): %v", email, err)
	}

	user, err := repo.GetUserByEmail(email)
	if err != nil {
		t.Fatalf("GetUserByEmail(%s): %v", email, err)
	}
	return *user
}

func TestUsersRepositoryRoundTrip(t *testing.T) {
	repo, _ := newUsersRepo(t)

	if has, err := repo.HasUsers(); err != nil || has {
		t.Fatalf("HasUsers() = %v, %v before any account, want false, nil", has, err)
	}

	created := createUser(t, repo, "alice@example.com", true)

	if has, err := repo.HasUsers(); err != nil || !has {
		t.Fatalf("HasUsers() = %v, %v after an account, want true, nil", has, err)
	}

	users, err := repo.GetUsers()
	if err != nil {
		t.Fatalf("GetUsers() error = %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("GetUsers() returned %d accounts, want 1", len(users))
	}

	got := users[0]
	if got.Email != "alice@example.com" {
		t.Errorf("email = %q, want alice@example.com", got.Email)
	}
	if string(got.Password) != "hash:alice@example.com" {
		t.Errorf("password = %q, want the bytes that were written", got.Password)
	}
	if !got.IsAdmin {
		t.Error("is_admin = false, want true: the flag did not round-trip")
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at is zero: the column default did not apply")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("updated_at is zero: the column default did not apply")
	}

	if byID, err := repo.GetUser(created.ID); err != nil || byID.Email != created.Email {
		t.Errorf("GetUser(id) = %+v, %v, want the created account", byID, err)
	}

	if _, err := repo.GetUser(uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetUser(missing) error = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetUserByEmail("nobody@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetUserByEmail(missing) error = %v, want ErrNotFound", err)
	}
}

// The UNIQUE on the email column is the invariant behind the signup race; the
// repository must surface it as an error rather than a second row.
func TestCreateUserRefusesDuplicateEmail(t *testing.T) {
	repo, _ := newUsersRepo(t)

	createUser(t, repo, "alice@example.com", false)

	if err := repo.CreateUser(User{Email: "alice@example.com", Password: []byte("other")}); err == nil {
		t.Fatal("CreateUser(duplicate email) error = nil, want the schema to refuse it")
	}
}

// An update changes the email and the password; no update path may grant the
// admin flag, because no request can.
func TestUpdateUserDoesNotChangeTheAdminFlag(t *testing.T) {
	repo, _ := newUsersRepo(t)

	user := createUser(t, repo, "alice@example.com", false)

	user.Email = "alice.new@example.com"
	user.Password = []byte("new-hash")
	user.IsAdmin = true // deliberately ignored by the query

	if err := repo.UpdateUser(user); err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}

	got, err := repo.GetUser(user.ID)
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}
	if got.Email != "alice.new@example.com" {
		t.Errorf("email = %q, want alice.new@example.com", got.Email)
	}
	if string(got.Password) != "new-hash" {
		t.Errorf("password = %q, want new-hash", got.Password)
	}
	if got.IsAdmin {
		t.Error("is_admin = true after an update: the flag is not an update field")
	}

	if err := repo.UpdateUser(User{ID: uuid.New(), Email: "x@example.com"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateUser(missing) error = %v, want ErrNotFound", err)
	}
}

// Deleting an account removes the rows that hang off it and reports the ids
// whose folders the caller must remove from disk (#144). Before migration
// 000014 every account that had ever logged in was impossible to delete.
func TestDeleteUserCascadesEveryRow(t *testing.T) {
	repo, db := newUsersRepo(t)

	alice := createUser(t, repo, "alice@example.com", false)
	bob := createUser(t, repo, "bob@example.com", false)

	fileID := uuid.New()
	db.MustExec(`INSERT INTO files (id, user_id, name, ext, path, content_type)
		VALUES ($1, $2, 'a.txt', 'txt', 'alice/1/1_a.txt', 'text/plain')`, fileID, alice.ID)
	db.MustExec(`INSERT INTO files_history (file_id, path, version) VALUES ($1, 'alice/1/1_a.txt', 1)`, fileID)
	db.MustExec(`INSERT INTO sessions (token, user_id, expires_at) VALUES ('alice-token', $1, now() + interval '1 hour')`, alice.ID)

	commandID := uuid.New()
	db.MustExec(`INSERT INTO commands (id, user_id, name) VALUES ($1, $2, 'fmt')`, commandID, alice.ID)

	templateID := uuid.New()
	db.MustExec(`INSERT INTO templates (id, user_id, name, slug) VALUES ($1, $2, 'mine', 'mine')`, templateID, alice.ID)

	owner, err := repo.DeleteUser(alice)
	if err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	if len(owner.TemplateIDs) != 1 || owner.TemplateIDs[0] != templateID {
		t.Errorf("TemplateIDs = %v, want [%s]", owner.TemplateIDs, templateID)
	}
	if len(owner.CommandIDs) != 1 || owner.CommandIDs[0] != commandID {
		t.Errorf("CommandIDs = %v, want [%s]", owner.CommandIDs, commandID)
	}

	if _, err := repo.GetUser(alice.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetUser(deleted) error = %v, want ErrNotFound", err)
	}

	// The cascades the migration added: sessions, files and their history.
	for _, check := range []struct {
		table     string
		condition string
	}{
		{"sessions", "user_id = $1"},
		{"files", "user_id = $1"},
		{"files_history", "file_id = $1"},
		{"commands", "user_id = $1"},
		{"templates", "user_id = $1"},
	} {
		id := alice.ID
		if check.table == "files_history" {
			id = fileID
		}
		if n := dbtest.Count(t, db, check.table, check.condition, id); n != 0 {
			t.Errorf("%s still has %d rows for the deleted account", check.table, n)
		}
	}

	// The other account is untouched.
	if _, err := repo.GetUser(bob.ID); err != nil {
		t.Errorf("GetUser(bob) error = %v, want the account to survive", err)
	}

	if _, err := repo.DeleteUser(alice); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteUser(already deleted) error = %v, want ErrNotFound", err)
	}
}

// A command an account owns can be refused by template_commands when another
// account's template pulls it in: the delete transaction must fail whole, not
// leave the account half-deleted.
func TestDeleteUserRollsBackWhenCommandIsInUse(t *testing.T) {
	repo, db := newUsersRepo(t)

	alice := createUser(t, repo, "alice@example.com", false)
	bob := createUser(t, repo, "bob@example.com", false)

	commandID := uuid.New()
	db.MustExec(`INSERT INTO commands (id, user_id, name) VALUES ($1, $2, 'fmt')`, commandID, alice.ID)

	templateID := uuid.New()
	db.MustExec(`INSERT INTO templates (id, user_id, name, slug) VALUES ($1, $2, 'bobs', 'bobs')`, templateID, bob.ID)
	db.MustExec(`INSERT INTO template_commands (template_id, command_id, target_path) VALUES ($1, $2, 'bin/fmt')`, templateID, commandID)

	inUse, err := repo.CommandsInUseByOthers(alice.ID)
	if err != nil {
		t.Fatalf("CommandsInUseByOthers() error = %v", err)
	}
	if !inUse {
		t.Fatal("CommandsInUseByOthers() = false, want true")
	}

	if _, err := repo.DeleteUser(alice); err == nil {
		t.Fatal("DeleteUser() error = nil, want the ON DELETE RESTRICT refusal")
	}

	if _, err := repo.GetUser(alice.ID); err != nil {
		t.Errorf("the account is gone after a refused delete: %v", err)
	}
	if n := dbtest.Count(t, db, "commands", "id = $1", commandID); n != 1 {
		t.Errorf("the command row = %d, want 1 after the rollback", n)
	}

	// Once the reference is gone the delete succeeds.
	db.MustExec(`DELETE FROM templates WHERE id = $1`, templateID)
	if _, err := repo.DeleteUser(alice); err != nil {
		t.Fatalf("DeleteUser() after the reference was removed: %v", err)
	}
}
