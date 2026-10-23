package templates

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

// newTestService returns a service backed by the given repository and a
// logger that writes to a throwaway folder.
func newTestService(t *testing.T, repo Repository) *service {
	t.Helper()

	service, _ := newTestServiceWithRoot(t, repo)

	return service
}

// newTestServiceWithRoot also returns the upload root the binary files of a
// template are written under, so a test can look at what landed on disk.
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

// A valid request, so a test can change the one thing it is about.
func validRequest() PublishRequest {
	return PublishRequest{
		Name: "Go project",
		Kind: KindGeneric,
		Files: []PublishFile{
			{Path: "AGENTS.md", Content: "# {{PROJECT_NAME}}"},
			{Path: ".opencode/agent.md", Content: "agent"},
		},
		Variables: []PublishVariable{
			{Key: "PROJECT_NAME", Prompt: "Project name", Required: true},
		},
	}
}

// A published template is stored with its files and its variables, and the
// detail the caller gets back carries all three.
func TestPublishTemplateStoresTheFilesAndTheVariables(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository()
	svc := newTestService(t, repo)

	detail, appError := svc.PublishTemplate(contextFor(owner), validRequest())
	if appError != nil {
		t.Fatalf("expected the template to be published, got error %v", appError)
	}

	if len(detail.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(detail.Files))
	}

	if len(detail.Variables) != 1 {
		t.Errorf("expected 1 variable, got %d", len(detail.Variables))
	}

	if detail.Version != 1 {
		t.Errorf("expected version 1, got %d", detail.Version)
	}

	if detail.Owner != owner.String() {
		t.Errorf("expected the template to belong to %s, got %s", owner, detail.Owner)
	}
}

// The files and the variables carry the id of the template they belong to.
//
// The row has a foreign key on it, and the database refuses a row without
// one — but the fake repository stores what it is given and never checks a
// key, so the rows the service builds are what the test has to look at.
func TestPublishTemplateGivesTheFilesAndTheVariablesTheTemplateID(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository()
	svc := newTestService(t, repo)

	detail, appError := svc.PublishTemplate(contextFor(owner), validRequest())
	if appError != nil {
		t.Fatalf("expected the template to be published, got error %v", appError)
	}

	files, ok := repo.GetTemplateFiles(detail.ID)
	if !ok {
		t.Fatal("expected the files to be stored")
	}

	if len(files) == 0 {
		t.Fatal("expected the files to be stored")
	}

	for _, file := range files {
		if file.TemplateID != detail.ID {
			t.Errorf("expected the file %q to belong to the template %s, got %s", file.Path, detail.ID, file.TemplateID)
		}
	}

	variables, ok := repo.GetTemplateVariables(detail.ID)
	if !ok {
		t.Fatal("expected the variables to be stored")
	}

	if len(variables) == 0 {
		t.Fatal("expected the variables to be stored")
	}

	for _, variable := range variables {
		if variable.TemplateID != detail.ID {
			t.Errorf("expected the variable %q to belong to the template %s, got %s", variable.Key, detail.ID, variable.TemplateID)
		}
	}
}

// The slug is derived from the name when the client sends none.
func TestPublishTemplateDerivesTheSlugFromTheName(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository()
	svc := newTestService(t, repo)

	detail, appError := svc.PublishTemplate(contextFor(owner), validRequest())
	if appError != nil {
		t.Fatalf("expected the template to be published, got error %v", appError)
	}

	if detail.Slug != "go-project" {
		t.Errorf("expected the slug %q, got %q", "go-project", detail.Slug)
	}
}

// Two templates with the same name do not fight over the slug: the second
// one gets a suffix.
func TestPublishTemplateGivesATakenSlugASuffix(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository()
	svc := newTestService(t, repo)

	first, appError := svc.PublishTemplate(contextFor(owner), validRequest())
	if appError != nil {
		t.Fatalf("expected the first template to be published, got error %v", appError)
	}

	second, appError := svc.PublishTemplate(contextFor(owner), validRequest())
	if appError != nil {
		t.Fatalf("expected the second template to be published, got error %v", appError)
	}

	if second.Slug == first.Slug {
		t.Errorf("expected the second slug to differ from %q", first.Slug)
	}
}

// A path that would write outside the project is refused.
func TestPublishTemplateRefusesAPathThatEscapes(t *testing.T) {
	owner := uuid.New()

	tests := []struct {
		name string
		path string
	}{
		{"parent", "../escape.md"},
		{"absolute", "/etc/passwd"},
		{"windows separator", `..\escape.md`},
		{"nested parent", "docs/../../escape.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository()
			svc := newTestService(t, repo)

			request := validRequest()
			request.Files = []PublishFile{{Path: tt.path, Content: "x"}}

			_, appError := svc.PublishTemplate(contextFor(owner), request)
			if appError == nil {
				t.Fatalf("expected the path %q to be refused", tt.path)
			}

			if appError.StatusCode() != http.StatusBadRequest {
				t.Errorf("expected status %d, got %d", http.StatusBadRequest, appError.StatusCode())
			}
		})
	}
}

// A kind the service does not know is refused: the value comes from a client.
func TestPublishTemplateRefusesAnUnknownKind(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository()
	svc := newTestService(t, repo)

	request := validRequest()
	request.Kind = "not-a-terminal"

	_, appError := svc.PublishTemplate(contextFor(owner), request)
	if appError == nil {
		t.Fatal("expected an unknown kind to be refused")
	}

	if appError.StatusCode() != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, appError.StatusCode())
	}
}

// A template without files is not a template.
func TestPublishTemplateRefusesAnEmptyTemplate(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository()
	svc := newTestService(t, repo)

	request := validRequest()
	request.Files = nil

	_, appError := svc.PublishTemplate(contextFor(owner), request)
	if appError == nil {
		t.Fatal("expected a template without files to be refused")
	}
}

// A file bigger than the cap is refused.
func TestPublishTemplateRefusesAFileThatIsTooBig(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository()
	svc := newTestService(t, repo)

	request := validRequest()
	request.Files = []PublishFile{{Path: "big.md", Content: string(make([]byte, maxTextFileBytes+1))}}

	_, appError := svc.PublishTemplate(contextFor(owner), request)
	if appError == nil {
		t.Fatal("expected the file to be refused")
	}
}

// The same path twice is a template that cannot be written: it is refused.
func TestPublishTemplateRefusesADuplicatePath(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository()
	svc := newTestService(t, repo)

	request := validRequest()
	request.Files = []PublishFile{
		{Path: "AGENTS.md", Content: "one"},
		{Path: "AGENTS.md", Content: "two"},
	}

	_, appError := svc.PublishTemplate(contextFor(owner), request)
	if appError == nil {
		t.Fatal("expected the duplicate path to be refused")
	}
}

// A variable key that is not a placeholder name is refused.
func TestPublishTemplateRefusesABadVariableKey(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository()
	svc := newTestService(t, repo)

	request := validRequest()
	request.Variables = []PublishVariable{{Key: "not a key", Prompt: "x"}}

	_, appError := svc.PublishTemplate(contextFor(owner), request)
	if appError == nil {
		t.Fatal("expected the variable key to be refused")
	}
}

// A private template of another user is not visible: the answer is the same
// as for a template that does not exist, so an id cannot be probed.
func TestGetTemplateHidesAPrivateTemplateOfAnotherUser(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()

	templateID := uuid.New()

	repo := newFakeRepository(Template{
		ID:       templateID,
		UserID:   owner,
		Name:     "secret",
		Slug:     "secret",
		Kind:     KindGeneric,
		IsPublic: false,
	})

	svc := newTestService(t, repo)

	_, appError := svc.GetTemplate(contextFor(stranger), templateID)
	if appError == nil {
		t.Fatal("expected the private template to be hidden")
	}

	if appError.StatusCode() != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, appError.StatusCode())
	}
}

// A public template is readable by anybody with a session.
func TestGetTemplateReturnsAPublicTemplate(t *testing.T) {
	owner := uuid.New()
	templateID := uuid.New()

	repo := newFakeRepository(Template{
		ID:       templateID,
		UserID:   owner,
		Name:     "shared",
		Slug:     "shared",
		Kind:     KindOpencode,
		IsPublic: true,
	})

	svc := newTestService(t, repo)

	detail, appError := svc.GetTemplate(contextFor(uuid.New()), templateID)
	if appError != nil {
		t.Fatalf("expected the public template to be returned, got error %v", appError)
	}

	if detail.Name != "shared" {
		t.Errorf("expected the template %q, got %q", "shared", detail.Name)
	}
}

// The catalogue holds the public templates and the caller's own, and nothing
// else: a private template of another user is not in it.
func TestListTemplatesHidesThePrivateTemplatesOfOtherUsers(t *testing.T) {
	owner := uuid.New()
	caller := uuid.New()

	repo := newFakeRepository(
		Template{ID: uuid.New(), UserID: owner, Name: "private", Slug: "private", Kind: KindGeneric},
		Template{ID: uuid.New(), UserID: owner, Name: "public", Slug: "public", Kind: KindGeneric, IsPublic: true},
		Template{ID: uuid.New(), UserID: caller, Name: "mine", Slug: "mine", Kind: KindGeneric},
	)

	svc := newTestService(t, repo)

	resp, appError := svc.ListTemplates(contextFor(caller), "")
	if appError != nil {
		t.Fatalf("expected the catalogue, got error %v", appError)
	}

	names := map[string]bool{}
	for _, template := range resp.Data {
		names[template.Name] = true
	}

	if names["private"] {
		t.Error("the private template of another user is in the catalogue")
	}

	if !names["public"] || !names["mine"] {
		t.Errorf("expected the public template and the caller's own, got %+v", names)
	}
}

// The catalogue can be filtered by kind, which is how a user finds the
// templates for the terminal they use.
func TestListTemplatesFiltersByKind(t *testing.T) {
	caller := uuid.New()

	repo := newFakeRepository(
		Template{ID: uuid.New(), UserID: caller, Name: "opencode one", Slug: "opencode-one", Kind: KindOpencode},
		Template{ID: uuid.New(), UserID: caller, Name: "codex one", Slug: "codex-one", Kind: KindCodex},
	)

	svc := newTestService(t, repo)

	resp, appError := svc.ListTemplates(contextFor(caller), KindCodex)
	if appError != nil {
		t.Fatalf("expected the catalogue, got error %v", appError)
	}

	if resp.Total != 1 {
		t.Fatalf("expected 1 template, got %d", resp.Total)
	}

	if resp.Data[0].Name != "codex one" {
		t.Errorf("expected the codex template, got %q", resp.Data[0].Name)
	}
}

// The catalogue carries the number of files a template holds: the count
// comes with the template, so the list and the detail page cannot disagree.
func TestListTemplatesCarriesTheFileCount(t *testing.T) {
	caller := uuid.New()

	repo := newFakeRepository(
		Template{ID: uuid.New(), UserID: caller, Name: "mine", Slug: "mine", Kind: KindGeneric, FileCount: 3},
	)

	svc := newTestService(t, repo)

	resp, appError := svc.ListTemplates(contextFor(caller), "")
	if appError != nil {
		t.Fatalf("expected the catalogue, got error %v", appError)
	}

	if resp.Data[0].FileCount != 3 {
		t.Errorf("expected 3 files, got %d", resp.Data[0].FileCount)
	}
}

// A filter that is not a kind is refused rather than ignored.
func TestListTemplatesRefusesAnUnknownKind(t *testing.T) {
	caller := uuid.New()

	svc := newTestService(t, newFakeRepository())

	_, appError := svc.ListTemplates(contextFor(caller), "not-a-terminal")
	if appError == nil {
		t.Fatal("expected an unknown kind to be refused")
	}
}

// Only the owner may change a template.
func TestUpdateTemplateRefusesAnotherUsersTemplate(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()
	templateID := uuid.New()

	repo := newFakeRepository(Template{
		ID:       templateID,
		UserID:   owner,
		Name:     "mine",
		Slug:     "mine",
		Kind:     KindGeneric,
		Version:  1,
		IsPublic: true,
	})

	svc := newTestService(t, repo)

	request := validRequest()
	request.Name = "hijacked"

	_, appError := svc.UpdateTemplate(contextFor(stranger), templateID, request)
	if appError == nil {
		t.Fatal("expected the update to be refused")
	}

	if appError.StatusCode() != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, appError.StatusCode())
	}

	stored, _ := repo.GetTemplate(templateID)
	if stored.Name != "mine" {
		t.Errorf("expected the template to be untouched, got %q", stored.Name)
	}
}

// The owner's update goes through, and the version goes up.
func TestUpdateTemplateBumpsTheVersion(t *testing.T) {
	owner := uuid.New()
	templateID := uuid.New()

	repo := newFakeRepository(Template{
		ID:       templateID,
		UserID:   owner,
		Name:     "mine",
		Slug:     "mine",
		Kind:     KindGeneric,
		Version:  1,
		IsPublic: true,
	})

	svc := newTestService(t, repo)

	request := validRequest()
	request.Name = "renamed"

	detail, appError := svc.UpdateTemplate(contextFor(owner), templateID, request)
	if appError != nil {
		t.Fatalf("expected the update to go through, got error %v", appError)
	}

	if detail.Version != 2 {
		t.Errorf("expected version 2, got %d", detail.Version)
	}

	if detail.Name != "renamed" {
		t.Errorf("expected the new name, got %q", detail.Name)
	}
}

// A template may keep its own slug across an update.
func TestUpdateTemplateKeepsTheSlugOfTheTemplate(t *testing.T) {
	owner := uuid.New()
	templateID := uuid.New()

	repo := newFakeRepository(Template{
		ID:       templateID,
		UserID:   owner,
		Name:     "mine",
		Slug:     "mine",
		Kind:     KindGeneric,
		Version:  1,
		IsPublic: true,
	})

	svc := newTestService(t, repo)

	detail, appError := svc.UpdateTemplate(contextFor(owner), templateID, validRequest())
	if appError != nil {
		t.Fatalf("expected the update to go through, got error %v", appError)
	}

	if detail.Slug != "mine" {
		t.Errorf("expected the slug %q, got %q", "mine", detail.Slug)
	}
}

// Only the owner may delete a template.
func TestDeleteTemplateRefusesAnotherUsersTemplate(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()
	templateID := uuid.New()

	repo := newFakeRepository(Template{
		ID:     templateID,
		UserID: owner,
		Name:   "mine",
		Slug:   "mine",
		Kind:   KindGeneric,
	})

	svc := newTestService(t, repo)

	_, appError := svc.DeleteTemplate(contextFor(stranger), templateID)
	if appError == nil {
		t.Fatal("expected the delete to be refused")
	}

	if _, ok := repo.GetTemplate(templateID); !ok {
		t.Error("the template was deleted by a user who does not own it")
	}
}

// The owner deletes their own template.
func TestDeleteTemplateRemovesTheOwnersTemplate(t *testing.T) {
	owner := uuid.New()
	templateID := uuid.New()

	repo := newFakeRepository(Template{
		ID:     templateID,
		UserID: owner,
		Name:   "mine",
		Slug:   "mine",
		Kind:   KindGeneric,
	})

	svc := newTestService(t, repo)

	if _, appError := svc.DeleteTemplate(contextFor(owner), templateID); appError != nil {
		t.Fatalf("expected the delete to go through, got error %v", appError)
	}

	if _, ok := repo.GetTemplate(templateID); ok {
		t.Error("the template was not deleted")
	}
}

// A request that never went through the guard is refused: the route is
// registered without it, and the service fails closed.
func TestTheServiceRefusesARequestWithoutAUser(t *testing.T) {
	svc := newTestService(t, newFakeRepository())

	_, appError := svc.ListTemplates(context.Background(), "")
	if appError == nil {
		t.Fatal("expected a request without a user to be refused")
	}

	if appError.StatusCode() != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, appError.StatusCode())
	}
}

// The slug of a name is lower case, with dashes between the words.
func TestSlugify(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "Go Project", "go-project"},
		{"spaces and symbols", "My  AI/Terminal!", "my-ai-terminal"},
		{"leading dash", "--name--", "name"},
		{"no letters", "123", "123"},
		{"empty", "", "template"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := slugify(tt.input); got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

// publishBinary publishes a template holding one binary file and returns the
// service, the root it writes to, the owner and the detail.
func publishBinary(t *testing.T, content []byte) (*service, string, uuid.UUID, *TemplateDetail) {
	t.Helper()

	repo := newFakeRepository()
	svc, root := newTestServiceWithRoot(t, repo)
	owner := uuid.New()

	request := validRequest()
	request.Files = []PublishFile{{
		Path:          "assets/logo.png",
		Kind:          FileKindBinary,
		ContentBase64: base64.StdEncoding.EncodeToString(content),
	}}

	detail, appError := svc.PublishTemplate(contextFor(owner), request)
	if appError != nil {
		t.Fatalf("expected the template to be published, got error %v", appError)
	}

	return svc, root, owner, detail
}

// A binary file is stored on disk, and the row points at it instead of
// carrying its bytes in a TEXT column.
func TestPublishTemplateStoresABinaryFileOnDisk(t *testing.T) {
	content := []byte{0x89, 'P', 'N', 'G', 0x00, 0xff}

	svc, root, owner, detail := publishBinary(t, content)

	if len(detail.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(detail.Files))
	}

	file := detail.Files[0]
	if file.Kind != FileKindBinary {
		t.Errorf("expected kind %q, got %q", FileKindBinary, file.Kind)
	}

	if file.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), file.Size)
	}

	if file.Content != "" {
		t.Errorf("expected no text content for a binary file, got %q", file.Content)
	}

	if file.ContentBase64 != base64.StdEncoding.EncodeToString(content) {
		t.Errorf("expected the detail to carry the bytes as base64, got %q", file.ContentBase64)
	}

	// The detail's base64 is what a client writes; the disk is what the
	// service reads back from.
	files, _ := svc.TemplateFiles(contextFor(owner), detail.ID)
	if len(files) != 1 {
		t.Fatalf("expected 1 stored file, got %d", len(files))
	}

	stored := files[0]
	want := "templates/" + detail.ID.String() + "/" + stored.ID.String() + "/logo.png"
	if stored.StoragePath != want {
		t.Errorf("expected storage path %q, got %q", want, stored.StoragePath)
	}

	absolute := filepath.Join(root, filepath.FromSlash(stored.StoragePath))
	got, err := os.ReadFile(absolute)
	if err != nil {
		t.Fatalf("expected the bytes on disk at %q: %v", absolute, err)
	}

	if string(got) != string(content) {
		t.Errorf("expected the bytes on disk to be %v, got %v", content, got)
	}
}

// A binary file may be bigger than the text cap: it is an image, not
// configuration.
func TestPublishTemplateAllowsABinaryFileOverTheTextCap(t *testing.T) {
	content := make([]byte, maxTextFileBytes+1)

	if _, _, _, detail := publishBinary(t, content); detail.Files[0].Size != int64(len(content)) {
		t.Errorf("expected the binary file to be stored, got size %d", detail.Files[0].Size)
	}
}

// A binary file without its bytes is refused: the row would point at nothing.
func TestPublishTemplateRefusesABinaryFileWithoutContent(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(t, repo)

	request := validRequest()
	request.Files = []PublishFile{{Path: "logo.png", Kind: FileKindBinary}}

	_, appError := svc.PublishTemplate(contextFor(uuid.New()), request)
	if appError == nil {
		t.Fatal("expected a binary file without content to be refused")
	}

	if appError.StatusCode() != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, appError.StatusCode())
	}
}

// Content that is not base64 is refused rather than written as bytes.
func TestPublishTemplateRefusesInvalidBase64(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(t, repo)

	request := validRequest()
	request.Files = []PublishFile{{Path: "logo.png", Kind: FileKindBinary, ContentBase64: "not base64!"}}

	_, appError := svc.PublishTemplate(contextFor(uuid.New()), request)
	if appError == nil {
		t.Fatal("expected invalid base64 to be refused")
	}
}

// A binary file carries content_base64 and no content; a text file the other
// way around. Mixing them is a client bug, and refusing it keeps the row and
// the disk from disagreeing.
func TestPublishTemplateRefusesTheWrongBodyForTheKind(t *testing.T) {
	tests := []struct {
		name string
		file PublishFile
	}{
		{"binary with content", PublishFile{Path: "logo.png", Kind: FileKindBinary, Content: "bytes?"}},
		{"text with base64", PublishFile{Path: "notes.md", Kind: FileKindText, ContentBase64: base64.StdEncoding.EncodeToString([]byte("bytes"))}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository()
			svc := newTestService(t, repo)

			request := validRequest()
			request.Files = []PublishFile{tt.file}

			if _, appError := svc.PublishTemplate(contextFor(uuid.New()), request); appError == nil {
				t.Fatalf("expected %s to be refused", tt.name)
			}
		})
	}
}

// An unknown kind is refused: the schema only knows two.
func TestPublishTemplateRefusesAnUnknownFileKind(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(t, repo)

	request := validRequest()
	request.Files = []PublishFile{{Path: "file.bin", Kind: "executable", Content: "x"}}

	_, appError := svc.PublishTemplate(contextFor(uuid.New()), request)
	if appError == nil {
		t.Fatal("expected an unknown kind to be refused")
	}
}

// A binary file over its own cap is refused before it is decoded.
func TestPublishTemplateRefusesABinaryFileThatIsTooBig(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(t, repo)

	// The encoded length alone is over the cap, so nothing is decoded.
	request := validRequest()
	request.Files = []PublishFile{{
		Path:          "huge.bin",
		Kind:          FileKindBinary,
		ContentBase64: strings.Repeat("A", maxBinaryFileBytes*4/3+8),
	}}

	_, appError := svc.PublishTemplate(contextFor(uuid.New()), request)
	if appError == nil {
		t.Fatal("expected the file to be refused")
	}
}

// An update replaces the files, and the bytes of the version it replaced are
// removed: nothing points at them any more.
func TestUpdateTemplateRemovesTheBytesItReplaced(t *testing.T) {
	svc, root, owner, detail := publishBinary(t, []byte("first"))

	oldPath := filepath.Join(root, filepath.FromSlash(templateStoragePathOf(t, svc, owner, detail)))
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("expected the first version on disk: %v", err)
	}

	request := validRequest()
	request.Files = []PublishFile{{
		Path:          "assets/new.png",
		Kind:          FileKindBinary,
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("second")),
	}}

	updated, appError := svc.UpdateTemplate(contextFor(owner), detail.ID, request)
	if appError != nil {
		t.Fatalf("expected the update to go through, got error %v", appError)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("expected the bytes of the replaced version to be removed, stat error: %v", err)
	}

	newPath := filepath.Join(root, filepath.FromSlash(templateStoragePathOf(t, svc, owner, updated)))
	got, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("expected the new version on disk at %q: %v", newPath, err)
	}

	if string(got) != "second" {
		t.Errorf("expected the new bytes, got %q", got)
	}
}

// Deleting a template removes its bytes with its rows.
func TestDeleteTemplateRemovesTheBytes(t *testing.T) {
	svc, root, owner, detail := publishBinary(t, []byte("bytes"))

	directory := filepath.Join(root, "templates", detail.ID.String())
	if _, err := os.Stat(directory); err != nil {
		t.Fatalf("expected the template folder on disk: %v", err)
	}

	if _, appError := svc.DeleteTemplate(contextFor(owner), detail.ID); appError != nil {
		t.Fatalf("expected the delete to go through, got error %v", appError)
	}

	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Errorf("expected the template folder to be removed, stat error: %v", err)
	}
}

// A publish the repository refuses leaves no bytes behind: the rows that
// would point at them were never written.
func TestPublishTemplateRemovesTheBytesWhenTheRepositoryFails(t *testing.T) {
	repo := newFakeRepository()
	repo.createFails = true

	svc, root := newTestServiceWithRoot(t, repo)

	request := validRequest()
	request.Files = []PublishFile{{
		Path:          "logo.png",
		Kind:          FileKindBinary,
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("bytes")),
	}}

	if _, appError := svc.PublishTemplate(contextFor(uuid.New()), request); appError == nil {
		t.Fatal("expected the publish to fail")
	}

	templates, err := os.ReadDir(filepath.Join(root, "templates"))
	if err == nil && len(templates) != 0 {
		t.Errorf("expected no template folder after a failed publish, found %d", len(templates))
	}
}

// The archive carries the bytes of a binary file, not its row.
func TestTemplateArchiveCarriesBinaryBytes(t *testing.T) {
	content := []byte{0x00, 0x01, 0xfe, 0xff}
	svc, _, owner, detail := publishBinary(t, content)

	entries, appError := svc.TemplateArchive(contextFor(owner), detail.ID)
	if appError != nil {
		t.Fatalf("expected the archive to be built, got error %v", appError)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if string(entries[0].Content) != string(content) {
		t.Errorf("expected the binary bytes in the archive, got %v", entries[0].Content)
	}
}

// One file of a template reads back as it was written, text or binary.
func TestTemplateFileContentReadsAFile(t *testing.T) {
	svc, _, owner, detail := publishBinary(t, []byte("binary bytes"))

	file, content, appError := svc.TemplateFileContent(contextFor(owner), detail.ID, detail.Files[0].ID)
	if appError != nil {
		t.Fatalf("expected the file to be read, got error %v", appError)
	}

	if file.Path != "assets/logo.png" {
		t.Errorf("expected the file path, got %q", file.Path)
	}

	if string(content) != "binary bytes" {
		t.Errorf("expected the bytes, got %q", content)
	}

	if _, _, appError := svc.TemplateFileContent(contextFor(owner), detail.ID, uuid.New()); appError == nil {
		t.Error("expected an unknown file to be refused")
	}
}

// A binary file of another user's private template is not found.
func TestTemplateFileContentRefusesAnotherUsersPrivateTemplate(t *testing.T) {
	svc, _, _, detail := publishBinary(t, []byte("bytes"))

	_, _, appError := svc.TemplateFileContent(contextFor(uuid.New()), detail.ID, detail.Files[0].ID)
	if appError == nil {
		t.Fatal("expected another user's private template to be refused")
	}

	if appError.StatusCode() != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, appError.StatusCode())
	}
}

// templateStoragePathOf returns the storage path of the only file of a
// detail, for a test that needs to look at the disk.
func templateStoragePathOf(t *testing.T, svc *service, owner uuid.UUID, detail *TemplateDetail) string {
	t.Helper()

	files, appError := svc.TemplateFiles(contextFor(owner), detail.ID)
	if appError != nil || len(files) != 1 {
		t.Fatalf("expected one stored file, got %d (error %v)", len(files), appError)
	}

	return files[0].StoragePath
}

// seedCommand puts a command in the repository's library, as if the user had
// created it, and returns it.
func seedCommand(repo *fakeRepository, owner uuid.UUID, name string, kind string) TemplateCommand {
	command := TemplateCommand{
		CommandID:    uuid.New(),
		OwnerID:      owner,
		Name:         name,
		Kind:         kind,
		Content:      "#!/bin/sh\n" + name,
		Size:         int64(len("#!/bin/sh\n" + name)),
		IsExecutable: true,
	}

	repo.addCommand(command)

	return command
}

// A template pulls a command in at a path, and the detail resolves the command
// from the owner's library.
func TestPublishTemplateStoresACommandReference(t *testing.T) {
	owner := uuid.New()
	repo := newFakeRepository()
	command := seedCommand(repo, owner, "fmt", FileKindText)

	svc := newTestService(t, repo)

	request := validRequest()
	request.Commands = []PublishCommand{{CommandID: command.CommandID, TargetPath: "scripts/fmt.sh"}}

	detail, appError := svc.PublishTemplate(contextFor(owner), request)
	if appError != nil {
		t.Fatalf("expected the template to be published, got error %v", appError)
	}

	if len(detail.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(detail.Commands))
	}

	pulled := detail.Commands[0]
	if pulled.TargetPath != "scripts/fmt.sh" {
		t.Errorf("expected the target path, got %q", pulled.TargetPath)
	}

	if pulled.CommandID != command.CommandID || pulled.Name != "fmt" {
		t.Errorf("expected the resolved command, got %+v", pulled)
	}

	if pulled.Content != command.Content || !pulled.IsExecutable {
		t.Errorf("expected the command content and its flag, got %+v", pulled)
	}
}

// A command of somebody else's library is refused as unknown: whether it
// exists is not the publisher's to learn.
func TestPublishTemplateRefusesAnUnknownCommand(t *testing.T) {
	owner := uuid.New()

	// A command of another user, referenced by its real id.
	repo := newFakeRepository()
	stranger := seedCommand(repo, uuid.New(), "theirs", FileKindText)

	tests := []struct {
		name    string
		command uuid.UUID
	}{
		{"a command that does not exist", uuid.New()},
		{"a command of another user", stranger.CommandID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t, repo)

			request := validRequest()
			request.Commands = []PublishCommand{{CommandID: tt.command, TargetPath: "scripts/fmt.sh"}}

			_, appError := svc.PublishTemplate(contextFor(owner), request)
			if appError == nil {
				t.Fatal("expected the command to be refused")
			}

			if appError.StatusCode() != http.StatusBadRequest {
				t.Errorf("expected status %d, got %d", http.StatusBadRequest, appError.StatusCode())
			}
		})
	}
}

// A template cannot put two things at one path: a command aimed where a file
// already lands, or two commands aimed at one path, would overwrite at apply
// time.
func TestPublishTemplateRefusesACollidingCommandPath(t *testing.T) {
	owner := uuid.New()

	tests := []struct {
		name     string
		commands func(first TemplateCommand, second TemplateCommand) []PublishCommand
	}{
		{"a command on a file path", func(first TemplateCommand, _ TemplateCommand) []PublishCommand {
			return []PublishCommand{{CommandID: first.CommandID, TargetPath: "AGENTS.md"}}
		}},
		{"two commands on one path", func(first TemplateCommand, second TemplateCommand) []PublishCommand {
			return []PublishCommand{
				{CommandID: first.CommandID, TargetPath: "scripts/run.sh"},
				{CommandID: second.CommandID, TargetPath: "scripts/run.sh"},
			}
		}},
		{"a command outside the project", func(first TemplateCommand, _ TemplateCommand) []PublishCommand {
			return []PublishCommand{{CommandID: first.CommandID, TargetPath: "../outside.sh"}}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository()
			first := seedCommand(repo, owner, "one", FileKindText)
			second := seedCommand(repo, owner, "two", FileKindText)

			svc := newTestService(t, repo)

			request := validRequest()
			request.Commands = tt.commands(first, second)

			if _, appError := svc.PublishTemplate(contextFor(owner), request); appError == nil {
				t.Fatalf("expected %s to be refused", tt.name)
			}
		})
	}
}

// The archive carries the commands at their target paths, text and binary.
func TestTemplateArchiveCarriesTheCommands(t *testing.T) {
	owner := uuid.New()
	repo := newFakeRepository()
	text := seedCommand(repo, owner, "fmt", FileKindText)

	binaryContent := []byte{0x7f, 'E', 'L', 'F', 0x00}
	binary := TemplateCommand{
		CommandID:    uuid.New(),
		OwnerID:      owner,
		Name:         "tool",
		Kind:         FileKindBinary,
		Size:         int64(len(binaryContent)),
		StoragePath:  "commands/" + uuid.New().String() + "/tool",
		IsExecutable: true,
	}
	repo.addCommand(binary)

	svc, root := newTestServiceWithRoot(t, repo)

	absolute := filepath.Join(root, filepath.FromSlash(binary.StoragePath))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatalf("failed to create the command folder: %v", err)
	}
	if err := os.WriteFile(absolute, binaryContent, 0o600); err != nil {
		t.Fatalf("failed to write the command bytes: %v", err)
	}

	request := validRequest()
	request.Files = []PublishFile{{Path: "AGENTS.md", Content: "# hello"}}
	request.Commands = []PublishCommand{
		{CommandID: text.CommandID, TargetPath: "scripts/fmt.sh"},
		{CommandID: binary.CommandID, TargetPath: "bin/tool"},
	}

	detail, appError := svc.PublishTemplate(contextFor(owner), request)
	if appError != nil {
		t.Fatalf("expected the template to be published, got error %v", appError)
	}

	entries, appError := svc.TemplateArchive(contextFor(owner), detail.ID)
	if appError != nil {
		t.Fatalf("expected the archive to be built, got error %v", appError)
	}

	byPath := map[string][]byte{}
	for _, entry := range entries {
		byPath[entry.Path] = entry.Content
	}

	if string(byPath["scripts/fmt.sh"]) != text.Content {
		t.Errorf("expected the text command at its target path, got %q", byPath["scripts/fmt.sh"])
	}

	if string(byPath["bin/tool"]) != string(binaryContent) {
		t.Errorf("expected the binary command bytes at its target path, got %v", byPath["bin/tool"])
	}
}

// The detail of a binary command carries its bytes, read from the disk the
// command's own row points at.
func TestTemplateDetailCarriesABinaryCommand(t *testing.T) {
	owner := uuid.New()
	repo := newFakeRepository()

	content := []byte{0x00, 0x01, 0xfe, 0xff}
	command := TemplateCommand{
		CommandID:    uuid.New(),
		OwnerID:      owner,
		Name:         "tool",
		Kind:         FileKindBinary,
		Size:         int64(len(content)),
		StoragePath:  "commands/" + uuid.New().String() + "/tool",
		IsExecutable: true,
	}
	repo.addCommand(command)

	svc, root := newTestServiceWithRoot(t, repo)

	absolute := filepath.Join(root, filepath.FromSlash(command.StoragePath))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatalf("failed to create the command folder: %v", err)
	}
	if err := os.WriteFile(absolute, content, 0o600); err != nil {
		t.Fatalf("failed to write the command bytes: %v", err)
	}

	request := validRequest()
	request.Commands = []PublishCommand{{CommandID: command.CommandID, TargetPath: "bin/tool"}}

	detail, appError := svc.PublishTemplate(contextFor(owner), request)
	if appError != nil {
		t.Fatalf("expected the template to be published, got error %v", appError)
	}

	if len(detail.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(detail.Commands))
	}

	if detail.Commands[0].Kind != FileKindBinary {
		t.Errorf("expected kind %q, got %q", FileKindBinary, detail.Commands[0].Kind)
	}

	if detail.Commands[0].ContentBase64 != base64.StdEncoding.EncodeToString(content) {
		t.Errorf("expected the bytes as base64, got %q", detail.Commands[0].ContentBase64)
	}
}

// An update replaces the references with the ones it carries: the ones the
// old version had are gone.
func TestUpdateTemplateReplacesTheCommands(t *testing.T) {
	owner := uuid.New()
	repo := newFakeRepository()
	command := seedCommand(repo, owner, "fmt", FileKindText)

	svc := newTestService(t, repo)

	request := validRequest()
	request.Commands = []PublishCommand{{CommandID: command.CommandID, TargetPath: "scripts/fmt.sh"}}

	detail, appError := svc.PublishTemplate(contextFor(owner), request)
	if appError != nil {
		t.Fatalf("expected the template to be published, got error %v", appError)
	}

	request.Commands = nil

	updated, appError := svc.UpdateTemplate(contextFor(owner), detail.ID, request)
	if appError != nil {
		t.Fatalf("expected the update to go through, got error %v", appError)
	}

	if len(updated.Commands) != 0 {
		t.Errorf("expected no commands after the update, got %+v", updated.Commands)
	}
}
