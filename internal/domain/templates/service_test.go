package templates

import (
	"context"
	"net/http"
	"testing"

	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
)

// newTestService returns a service backed by the given repository and a
// logger that writes to a throwaway folder.
func newTestService(t *testing.T, repo Repository) *service {
	t.Helper()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	return NewService(repo, log)
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
	request.Files = []PublishFile{{Path: "big.md", Content: string(make([]byte, maxFileBytes+1))}}

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
