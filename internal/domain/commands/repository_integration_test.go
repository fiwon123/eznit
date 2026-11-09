//go:build integration

package commands

import (
	"errors"
	"testing"

	"github.com/fiwon123/eznit/internal/platform/dbtest"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func newCommands(t *testing.T) (*sqlRepository, *sqlx.DB) {
	t.Helper()

	db := dbtest.Open(t)
	dbtest.Reset(t, db)

	return NewRepository(db, dbtest.Logger(t)), db
}

func createAccount(t *testing.T, db *sqlx.DB, email string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	if err := db.Get(&id,
		"INSERT INTO users (email, password) VALUES ($1, $2) RETURNING id",
		email, []byte("hash")); err != nil {
		t.Fatalf("create account %s: %v", email, err)
	}
	return id
}

func TestCommandsCRUD(t *testing.T) {
	repo, db := newCommands(t)

	alice := createAccount(t, db, "alice@example.com")
	bob := createAccount(t, db, "bob@example.com")

	text := Command{
		ID:           uuid.New(),
		UserID:       alice,
		Name:         "fmt",
		Kind:         "text",
		Content:      "#!/bin/sh\ngofmt -w .\n",
		Size:         20,
		IsExecutable: true,
	}
	binary := Command{
		ID:           uuid.New(),
		UserID:       alice,
		Name:         "lint",
		Kind:         "binary",
		StoragePath:  "commands/" + uuid.NewString() + "/lint",
		Size:         4096,
		IsExecutable: true,
	}

	for _, command := range []Command{text, binary} {
		if err := repo.CreateCommand(command); err != nil {
			t.Fatalf("CreateCommand(%s): %v", command.Name, err)
		}
	}

	// Ordered by name, so a listing is stable between reads.
	list, err := repo.ListCommands(alice)
	if err != nil {
		t.Fatalf("ListCommands() error = %v", err)
	}
	if len(list) != 2 || list[0].Name != "fmt" || list[1].Name != "lint" {
		t.Fatalf("ListCommands() = %v, want fmt then lint", list)
	}

	got, err := repo.GetCommand(text.ID, alice)
	if err != nil {
		t.Fatalf("GetCommand() error = %v", err)
	}
	if got.Content != text.Content || !got.IsExecutable {
		t.Errorf("command = %+v, want the values that were written", got)
	}

	byName, err := repo.GetCommandByName(alice, "lint")
	if err != nil || byName.ID != binary.ID {
		t.Errorf("GetCommandByName(lint) = %+v, %v, want the binary command", byName, err)
	}

	if _, err := repo.GetCommand(text.ID, bob); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetCommand(other account) error = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetCommandByName(alice, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetCommandByName(missing) error = %v, want ErrNotFound", err)
	}

	text.Name = "format"
	text.Content = "#!/bin/sh\nshfmt -w .\n"
	if err := repo.UpdateCommand(text); err != nil {
		t.Fatalf("UpdateCommand() error = %v", err)
	}
	updated, err := repo.GetCommand(text.ID, alice)
	if err != nil {
		t.Fatalf("GetCommand() after update error = %v", err)
	}
	if updated.Name != "format" || updated.Content != text.Content {
		t.Errorf("command after update = %+v, want the new name and content", updated)
	}

	if err := repo.UpdateCommand(Command{ID: uuid.New(), UserID: alice, Name: "ghost", Kind: "text"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateCommand(missing) error = %v, want ErrNotFound", err)
	}

	if err := repo.DeleteCommand(text.ID, bob); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteCommand(other account) error = %v, want ErrNotFound", err)
	}
	if err := repo.DeleteCommand(text.ID, alice); err != nil {
		t.Fatalf("DeleteCommand(owner) error = %v", err)
	}
	if _, err := repo.GetCommand(text.ID, alice); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetCommand(deleted) error = %v, want ErrNotFound", err)
	}
}

// A name is unique per user, not globally: two accounts may both own `fmt`
// (migration 000011).
func TestCommandNameIsUniquePerUser(t *testing.T) {
	repo, db := newCommands(t)

	alice := createAccount(t, db, "alice@example.com")
	bob := createAccount(t, db, "bob@example.com")

	aliceFmt := Command{ID: uuid.New(), UserID: alice, Name: "fmt", Kind: "text", Content: "a"}
	if err := repo.CreateCommand(aliceFmt); err != nil {
		t.Fatalf("CreateCommand(alice) error = %v", err)
	}

	if err := repo.CreateCommand(Command{ID: uuid.New(), UserID: alice, Name: "fmt", Kind: "text", Content: "b"}); err == nil {
		t.Fatal("CreateCommand(duplicate name, same user) error = nil, want the UNIQUE refusal")
	}

	if err := repo.CreateCommand(Command{ID: uuid.New(), UserID: bob, Name: "fmt", Kind: "text", Content: "c"}); err != nil {
		t.Fatalf("CreateCommand(same name, other user) error = %v: the name is scoped to the owner", err)
	}
}

// template_commands is ON DELETE RESTRICT: a command a published template pulls
// in cannot be deleted until the reference is gone (#144).
func TestCommandInUseIsRefused(t *testing.T) {
	repo, db := newCommands(t)

	alice := createAccount(t, db, "alice@example.com")
	bob := createAccount(t, db, "bob@example.com")

	command := Command{ID: uuid.New(), UserID: alice, Name: "fmt", Kind: "text", Content: "#!/bin/sh\n"}
	if err := repo.CreateCommand(command); err != nil {
		t.Fatalf("CreateCommand() error = %v", err)
	}

	templateID := uuid.New()
	if _, err := db.Exec(
		"INSERT INTO templates (id, user_id, name, slug) VALUES ($1, $2, 'bobs', 'bobs')",
		templateID, bob); err != nil {
		t.Fatalf("create template: %v", err)
	}
	if _, err := db.Exec(
		"INSERT INTO template_commands (template_id, command_id, target_path) VALUES ($1, $2, 'bin/fmt')",
		templateID, command.ID); err != nil {
		t.Fatalf("create reference: %v", err)
	}

	inUse, err := repo.CommandInUse(command.ID)
	if err != nil || !inUse {
		t.Fatalf("CommandInUse() = %v, %v, want true", inUse, err)
	}

	if err := repo.DeleteCommand(command.ID, alice); err == nil {
		t.Fatal("DeleteCommand(in use) error = nil, want the ON DELETE RESTRICT refusal")
	}

	if _, err := db.Exec("DELETE FROM templates WHERE id = $1", templateID); err != nil {
		t.Fatalf("delete the template: %v", err)
	}

	inUse, err = repo.CommandInUse(command.ID)
	if err != nil || inUse {
		t.Fatalf("CommandInUse() after the reference was removed = %v, %v, want false", inUse, err)
	}

	if err := repo.DeleteCommand(command.ID, alice); err != nil {
		t.Fatalf("DeleteCommand() after the reference was removed: %v", err)
	}
}
