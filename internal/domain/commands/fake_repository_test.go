package commands

import (
	"errors"
	"sort"

	"github.com/google/uuid"
)

// fakeRepository is an in-memory Repository. The rules under test — what a
// valid command is, whose it is, and what a delete must refuse — live in the
// service, so the tests drive the service with a repository that needs no
// database.
type fakeRepository struct {
	commands map[uuid.UUID]Command
	inUse    map[uuid.UUID]bool
	// createFails makes the write fail, for the test that checks what the
	// service does with the bytes when the row is not written.
	createFails bool
}

func newFakeRepository(commands ...Command) *fakeRepository {
	repo := &fakeRepository{
		commands: map[uuid.UUID]Command{},
		inUse:    map[uuid.UUID]bool{},
	}

	for _, command := range commands {
		repo.commands[command.ID] = command
	}

	return repo
}

func (r *fakeRepository) ListCommands(userID uuid.UUID) ([]Command, error) {
	var owned []Command

	for _, command := range r.commands {
		if command.UserID == userID {
			owned = append(owned, command)
		}
	}

	sort.Slice(owned, func(i, j int) bool { return owned[i].Name < owned[j].Name })

	return owned, nil
}

func (r *fakeRepository) GetCommand(id uuid.UUID, userID uuid.UUID) (*Command, error) {
	command, ok := r.commands[id]
	if !ok || command.UserID != userID {
		return nil, ErrNotFound
	}

	return &command, nil
}

func (r *fakeRepository) GetCommandByName(userID uuid.UUID, name string) (*Command, error) {
	for _, command := range r.commands {
		if command.UserID == userID && command.Name == name {
			return &command, nil
		}
	}

	return nil, ErrNotFound
}

func (r *fakeRepository) CreateCommand(command Command) error {
	if r.createFails {
		return errors.New("create failed")
	}

	r.commands[command.ID] = command

	return nil
}

func (r *fakeRepository) UpdateCommand(command Command) error {
	if _, ok := r.commands[command.ID]; !ok {
		return ErrNotFound
	}

	r.commands[command.ID] = command

	return nil
}

func (r *fakeRepository) DeleteCommand(id uuid.UUID, userID uuid.UUID) error {
	command, ok := r.commands[id]
	if !ok || command.UserID != userID {
		return ErrNotFound
	}

	delete(r.commands, id)

	return nil
}

func (r *fakeRepository) CommandInUse(id uuid.UUID) (bool, error) {
	return r.inUse[id], nil
}
