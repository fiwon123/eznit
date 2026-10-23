package commands

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
)

func newTestService(t *testing.T, repo Repository) *service {
	t.Helper()

	service, _ := newTestServiceWithRoot(t, repo)

	return service
}

// newTestServiceWithRoot also returns the upload root the bytes of a binary
// command are written under, so a test can look at what landed on disk.
func newTestServiceWithRoot(t *testing.T, repo Repository) (*service, string) {
	t.Helper()

	root := t.TempDir()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	return NewService(repo, root, log), root
}

// contextFor returns a context with the user the guard would have set.
func contextFor(userID uuid.UUID) context.Context {
	return context.WithValue(context.Background(), "user_id", userID)
}

// A text command is stored with its content and it is executable by default:
// a command is a thing you run.
func TestCreateCommandStoresATextCommand(t *testing.T) {
	owner := uuid.New()
	svc := newTestService(t, newFakeRepository())

	data, appError := svc.CreateCommand(contextFor(owner), CommandRequest{
		Name:    "fmt",
		Content: "#!/bin/sh\ngofmt -w .",
	})
	if appError != nil {
		t.Fatalf("expected the command to be created, got error %v", appError)
	}

	if data.Kind != CommandKindText {
		t.Errorf("expected kind %q, got %q", CommandKindText, data.Kind)
	}

	if data.Content != "#!/bin/sh\ngofmt -w ." {
		t.Errorf("expected the content to be stored, got %q", data.Content)
	}

	if data.Size != int64(len("#!/bin/sh\ngofmt -w .")) {
		t.Errorf("expected size %d, got %d", len("#!/bin/sh\ngofmt -w ."), data.Size)
	}

	if !data.IsExecutable {
		t.Error("expected a command to be executable by default")
	}
}

// A binary command is written under the upload root, and the row points at
// it instead of carrying its bytes in a TEXT column.
func TestCreateCommandStoresABinaryCommandOnDisk(t *testing.T) {
	owner := uuid.New()
	content := []byte{0x7f, 'E', 'L', 'F', 0x00, 0xff}

	svc, root := newTestServiceWithRoot(t, newFakeRepository())

	data, appError := svc.CreateCommand(contextFor(owner), CommandRequest{
		Name:          "tool",
		Kind:          CommandKindBinary,
		ContentBase64: base64.StdEncoding.EncodeToString(content),
	})
	if appError != nil {
		t.Fatalf("expected the command to be created, got error %v", appError)
	}

	if data.Content != "" {
		t.Errorf("expected no text content for a binary command, got %q", data.Content)
	}

	if data.ContentBase64 != base64.StdEncoding.EncodeToString(content) {
		t.Errorf("expected the bytes back as base64, got %q", data.ContentBase64)
	}

	absolute := filepath.Join(root, "commands", data.ID.String(), "tool")

	got, err := os.ReadFile(absolute)
	if err != nil {
		t.Fatalf("expected the bytes on disk at %q: %v", absolute, err)
	}

	if string(got) != string(content) {
		t.Errorf("expected the bytes %v, got %v", content, got)
	}
}

// A name is how a user refers to a command, so it has to be there and it has
// to be a label rather than a path.
func TestCreateCommandRefusesABadName(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"only spaces", "   "},
		{"a separator", "bin/tool"},
		{"a dot-dot", ".."},
		{"too long", strings.Repeat("a", maxNameLength+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t, newFakeRepository())

			_, appError := svc.CreateCommand(contextFor(uuid.New()), CommandRequest{Name: tt.input, Content: "x"})
			if appError == nil {
				t.Fatalf("expected the name %q to be refused", tt.input)
			}

			if appError.StatusCode() != http.StatusBadRequest {
				t.Errorf("expected status %d, got %d", http.StatusBadRequest, appError.StatusCode())
			}
		})
	}
}

// Two commands of one user cannot share a name: the name is the lookup.
func TestCreateCommandRefusesADuplicateName(t *testing.T) {
	owner := uuid.New()
	svc := newTestService(t, newFakeRepository())

	if _, appError := svc.CreateCommand(contextFor(owner), CommandRequest{Name: "fmt", Content: "x"}); appError != nil {
		t.Fatalf("expected the first command to be created, got error %v", appError)
	}

	_, appError := svc.CreateCommand(contextFor(owner), CommandRequest{Name: "fmt", Content: "y"})
	if appError == nil {
		t.Fatal("expected the duplicate name to be refused")
	}

	if appError.StatusCode() != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, appError.StatusCode())
	}
}

// Two users may both own a command called `fmt`: the library is per user.
func TestCreateCommandAllowsTheSameNameForAnotherUser(t *testing.T) {
	svc := newTestService(t, newFakeRepository())

	if _, appError := svc.CreateCommand(contextFor(uuid.New()), CommandRequest{Name: "fmt", Content: "x"}); appError != nil {
		t.Fatalf("expected the first command to be created, got error %v", appError)
	}

	if _, appError := svc.CreateCommand(contextFor(uuid.New()), CommandRequest{Name: "fmt", Content: "y"}); appError != nil {
		t.Fatalf("expected the second user's command to be created, got error %v", appError)
	}
}

// The body has to match the kind: a binary command carries base64, a text one
// carries text, and neither carries both.
func TestCreateCommandRefusesTheWrongBodyForTheKind(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("bytes"))

	tests := []struct {
		name    string
		request CommandRequest
	}{
		{"binary without content", CommandRequest{Name: "tool", Kind: CommandKindBinary}},
		{"binary with text", CommandRequest{Name: "tool", Kind: CommandKindBinary, Content: "text"}},
		{"binary with invalid base64", CommandRequest{Name: "tool", Kind: CommandKindBinary, ContentBase64: "not base64!"}},
		{"text with base64", CommandRequest{Name: "fmt", Kind: CommandKindText, ContentBase64: encoded}},
		{"an unknown kind", CommandRequest{Name: "fmt", Kind: "executable", Content: "x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t, newFakeRepository())

			if _, appError := svc.CreateCommand(contextFor(uuid.New()), tt.request); appError == nil {
				t.Fatalf("expected %s to be refused", tt.name)
			}
		})
	}
}

// Both caps are enforced by the server, not only by the client.
func TestCreateCommandRefusesACommandThatIsTooBig(t *testing.T) {
	tests := []struct {
		name    string
		request CommandRequest
	}{
		{"a text command over its cap", CommandRequest{Name: "fmt", Content: strings.Repeat("a", maxTextBytes+1)}},
		{
			"a binary command over its cap",
			CommandRequest{
				Name:          "tool",
				Kind:          CommandKindBinary,
				ContentBase64: base64.StdEncoding.EncodeToString(make([]byte, maxBinaryBytes+1)),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t, newFakeRepository())

			if _, appError := svc.CreateCommand(contextFor(uuid.New()), tt.request); appError == nil {
				t.Fatalf("expected %s to be refused", tt.name)
			}
		})
	}
}

// Another user's command is not found, not forbidden: its existence is not
// the caller's to learn.
func TestGetCommandHidesAnotherUsersCommand(t *testing.T) {
	owner := uuid.New()
	commandID := uuid.New()

	svc := newTestService(t, newFakeRepository(Command{ID: commandID, UserID: owner, Name: "fmt", Kind: CommandKindText}))

	_, appError := svc.GetCommand(contextFor(uuid.New()), commandID)
	if appError == nil {
		t.Fatal("expected another user's command to be refused")
	}

	if appError.StatusCode() != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, appError.StatusCode())
	}
}

// The listing is the caller's library, not everybody's.
func TestListCommandsReturnsOnlyTheCallersLibrary(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()

	svc := newTestService(t, newFakeRepository(
		Command{ID: uuid.New(), UserID: owner, Name: "mine", Kind: CommandKindText},
		Command{ID: uuid.New(), UserID: stranger, Name: "theirs", Kind: CommandKindText},
	))

	data, appError := svc.ListCommands(contextFor(owner))
	if appError != nil {
		t.Fatalf("expected the listing to work, got error %v", appError)
	}

	if data.Total != 1 || data.Data[0].Name != "mine" {
		t.Errorf("expected only the caller's command, got %+v", data.Data)
	}
}

// An update replaces the bytes of the version it replaced.
func TestUpdateCommandRemovesTheBytesItReplaced(t *testing.T) {
	owner := uuid.New()

	svc, root := newTestServiceWithRoot(t, newFakeRepository())

	created, appError := svc.CreateCommand(contextFor(owner), CommandRequest{
		Name:          "tool",
		Kind:          CommandKindBinary,
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("first")),
	})
	if appError != nil {
		t.Fatalf("expected the command to be created, got error %v", appError)
	}

	oldPath := filepath.Join(root, "commands", created.ID.String(), "tool")

	updated, appError := svc.UpdateCommand(contextFor(owner), created.ID, CommandRequest{
		Name:          "tool2",
		Kind:          CommandKindBinary,
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("second")),
	})
	if appError != nil {
		t.Fatalf("expected the update to go through, got error %v", appError)
	}

	if updated.Size != int64(len("second")) {
		t.Errorf("expected size %d, got %d", len("second"), updated.Size)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("expected the bytes of the replaced name to be removed, stat error: %v", err)
	}

	newPath := filepath.Join(root, "commands", created.ID.String(), "tool2")

	got, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("expected the new bytes on disk at %q: %v", newPath, err)
	}

	if string(got) != "second" {
		t.Errorf("expected the new bytes, got %q", got)
	}
}

// A command the library entry for a template cannot simply disappear: the
// published template would lose a file. The delete is refused.
func TestDeleteCommandRefusesACommandATemplateUses(t *testing.T) {
	owner := uuid.New()
	commandID := uuid.New()

	repo := newFakeRepository(Command{ID: commandID, UserID: owner, Name: "fmt", Kind: CommandKindText})
	repo.inUse[commandID] = true

	svc := newTestService(t, repo)

	_, appError := svc.DeleteCommand(contextFor(owner), commandID)
	if appError == nil {
		t.Fatal("expected the delete to be refused")
	}

	if appError.StatusCode() != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, appError.StatusCode())
	}

	if _, ok := repo.commands[commandID]; !ok {
		t.Error("expected the command to survive the refused delete")
	}
}

// Deleting a command removes it and its bytes.
func TestDeleteCommandRemovesTheBytes(t *testing.T) {
	owner := uuid.New()

	svc, root := newTestServiceWithRoot(t, newFakeRepository())

	created, appError := svc.CreateCommand(contextFor(owner), CommandRequest{
		Name:          "tool",
		Kind:          CommandKindBinary,
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("bytes")),
	})
	if appError != nil {
		t.Fatalf("expected the command to be created, got error %v", appError)
	}

	directory := filepath.Join(root, "commands", created.ID.String())
	if _, err := os.Stat(directory); err != nil {
		t.Fatalf("expected the command folder on disk: %v", err)
	}

	if _, appError := svc.DeleteCommand(contextFor(owner), created.ID); appError != nil {
		t.Fatalf("expected the delete to go through, got error %v", appError)
	}

	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Errorf("expected the command folder to be removed, stat error: %v", err)
	}
}

// A create the repository refuses leaves no bytes behind: the row that would
// point at them was never written.
func TestCreateCommandRemovesTheBytesWhenTheRepositoryFails(t *testing.T) {
	repo := newFakeRepository()
	repo.createFails = true

	svc, root := newTestServiceWithRoot(t, repo)

	if _, appError := svc.CreateCommand(contextFor(uuid.New()), CommandRequest{
		Name:          "tool",
		Kind:          CommandKindBinary,
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("bytes")),
	}); appError == nil {
		t.Fatal("expected the create to fail")
	}

	commands, err := os.ReadDir(filepath.Join(root, "commands"))
	if err == nil && len(commands) != 0 {
		t.Errorf("expected no command folder after a failed create, found %d", len(commands))
	}
}

// Without a user on the context — the guard did not run, or the service was
// called from code that forgot it — nothing is read or written.
func TestCommandsNeedAUser(t *testing.T) {
	svc := newTestService(t, newFakeRepository())

	if _, appError := svc.ListCommands(context.Background()); appError == nil || appError.StatusCode() != http.StatusUnauthorized {
		t.Errorf("expected the listing to need a user, got %v", appError)
	}

	if _, appError := svc.CreateCommand(context.Background(), CommandRequest{Name: "fmt", Content: "x"}); appError == nil || appError.StatusCode() != http.StatusUnauthorized {
		t.Errorf("expected the create to need a user, got %v", appError)
	}
}
