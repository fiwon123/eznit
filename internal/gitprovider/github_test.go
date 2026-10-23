package gitprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeHost is a GitHub-shaped server. Nothing in these tests reaches the
// network: every request goes to a local handler.
func fakeHost(t *testing.T, handler http.HandlerFunc) (*GitHub, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return NewGitHub(server.URL), server
}

// The token travels in the Authorization header, not in the URL.
func TestViewerSendsTheTokenInTheHeader(t *testing.T) {
	var gotAuth, gotAccept, gotAgent string

	provider, _ := fakeHost(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotAgent = r.Header.Get("User-Agent")

		if r.URL.Path != "/user" {
			t.Errorf("expected /user, got %s", r.URL.Path)
		}

		if strings.Contains(r.URL.RawQuery, "the-token") {
			t.Errorf("the token is in the URL: %q", r.URL.RawQuery)
		}

		_, _ = w.Write([]byte(`{"login":"octocat"}`))
	})

	login, err := provider.Viewer(context.Background(), "the-token")
	if err != nil {
		t.Fatalf("expected the viewer to be read, got %v", err)
	}

	if login != "octocat" {
		t.Errorf("expected octocat, got %q", login)
	}

	if gotAuth != "Bearer the-token" {
		t.Errorf("expected the bearer header, got %q", gotAuth)
	}

	if gotAccept != "application/vnd.github+json" {
		t.Errorf("expected the GitHub accept header, got %q", gotAccept)
	}

	if gotAgent != "eznit" {
		t.Errorf("expected a User-Agent, got %q", gotAgent)
	}
}

// A listing follows pages until a page is short, and it keeps the order the
// host sends.
func TestListRepositoriesReadsEveryPage(t *testing.T) {
	var pages []string

	provider, _ := fakeHost(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages = append(pages, page)

		repositories := []githubRepository{}

		if page == "1" {
			for i := 0; i < pageSize; i++ {
				repositories = append(repositories, githubRepository{
					FullName: fmt.Sprintf("owner/repo-%03d", i),
					Name:     fmt.Sprintf("repo-%03d", i),
					CloneURL: "https://example.test/owner/repo.git",
				})
				repositories[i].Owner.Login = "owner"
			}
		} else {
			repositories = append(repositories, githubRepository{FullName: "owner/last"})
			repositories[0].Owner.Login = "owner"
		}

		_ = json.NewEncoder(w).Encode(repositories)
	})

	repositories, err := provider.ListRepositories(context.Background(), "the-token")
	if err != nil {
		t.Fatalf("expected the listing to work, got %v", err)
	}

	if len(repositories) != pageSize+1 {
		t.Fatalf("expected %d repositories, got %d", pageSize+1, len(repositories))
	}

	if len(pages) != 2 || pages[0] != "1" || pages[1] != "2" {
		t.Errorf("expected two pages, got %v", pages)
	}

	if repositories[0].FullName != "owner/repo-000" || repositories[0].Owner != "owner" {
		t.Errorf("expected the first repository, got %+v", repositories[0])
	}

	if repositories[len(repositories)-1].FullName != "owner/last" {
		t.Errorf("expected the last repository, got %+v", repositories[len(repositories)-1])
	}
}

// A clone URL and the private flag come through: they are what the CLI needs
// to clone without building a URL of its own.
func TestListRepositoriesKeepsTheCloneURL(t *testing.T) {
	provider, _ := fakeHost(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{
			"full_name": "owner/secret",
			"name": "secret",
			"private": true,
			"clone_url": "https://example.test/owner/secret.git",
			"default_branch": "main",
			"owner": {"login": "owner"}
		}]`))
	})

	repositories, err := provider.ListRepositories(context.Background(), "the-token")
	if err != nil {
		t.Fatalf("expected the listing to work, got %v", err)
	}

	if len(repositories) != 1 {
		t.Fatalf("expected 1 repository, got %d", len(repositories))
	}

	repository := repositories[0]

	if !repository.Private || repository.CloneURL != "https://example.test/owner/secret.git" || repository.DefaultBranch != "main" {
		t.Errorf("expected the wire fields to survive, got %+v", repository)
	}
}

// Every way a host refuses a request has its own answer, because the CLI says
// something different for each.
func TestTheHostErrorsAreNamed(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		headers map[string]string
		want    error
	}{
		{"a refused token", http.StatusUnauthorized, nil, ErrUnauthorized},
		{"a missing repository", http.StatusNotFound, nil, ErrNotFound},
		{"a rate limit", http.StatusForbidden, map[string]string{"X-RateLimit-Remaining": "0"}, ErrRateLimited},
		{"too many requests", http.StatusTooManyRequests, nil, ErrRateLimited},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, _ := fakeHost(t, func(w http.ResponseWriter, r *http.Request) {
				for key, value := range tt.headers {
					w.Header().Set(key, value)
				}

				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"message":"nope"}`))
			})

			_, err := provider.ListRepositories(context.Background(), "the-token")
			if !errors.Is(err, tt.want) {
				t.Errorf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

// A 403 that is not a rate limit is a permission problem, not a "wait".
func TestAPermissionErrorIsNotARateLimit(t *testing.T) {
	provider, _ := fakeHost(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "42")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible"}`))
	})

	_, err := provider.ListRepositories(context.Background(), "the-token")
	if err == nil {
		t.Fatal("expected an error")
	}

	if errors.Is(err, ErrRateLimited) {
		t.Errorf("expected a permission error, got a rate limit: %v", err)
	}

	if !strings.Contains(err.Error(), "Resource not accessible") {
		t.Errorf("expected the host's message, got %v", err)
	}
}

// A host that answers with something that is not JSON is an error, not an
// empty listing.
func TestListRepositoriesRefusesANonJSONAnswer(t *testing.T) {
	provider, _ := fakeHost(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>maintenance</html>"))
	})

	if _, err := provider.ListRepositories(context.Background(), "the-token"); err == nil {
		t.Fatal("expected the answer to be refused")
	}
}

// One repository is read from its own route, with both parts of the name
// escaped into their own segment.
func TestGetRepositoryEscapesTheName(t *testing.T) {
	var gotPath string

	provider, _ := fakeHost(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"full_name":"owner/repo","name":"repo","owner":{"login":"owner"},"clone_url":"https://example.test/owner/repo.git"}`))
	})

	repository, err := provider.GetRepository(context.Background(), "the-token", "owner/repo")
	if err != nil {
		t.Fatalf("expected the repository to be read, got %v", err)
	}

	if gotPath != "/repos/owner/repo" {
		t.Errorf("expected /repos/owner/repo, got %q", gotPath)
	}

	if repository.FullName != "owner/repo" {
		t.Errorf("expected the repository, got %+v", repository)
	}
}

// A name that is not `<owner>/<name>` never reaches the host: it cannot be a
// path, so it is not a repository.
func TestGetRepositoryRefusesABadNameWithoutARequest(t *testing.T) {
	called := false

	provider, _ := fakeHost(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	for _, name := range []string{"", "owner", "owner/repo/extra", "../repo", "owner/..", "owner/re po"} {
		if _, err := provider.GetRepository(context.Background(), "the-token", name); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q: expected ErrNotFound, got %v", name, err)
		}
	}

	if called {
		t.Error("expected no request for an invalid name")
	}
}

// The git header is HTTP Basic with the token as the password, and the token
// is not readable in the key the CLI passes around.
func TestGitAuthHeader(t *testing.T) {
	provider := NewGitHub("")

	key, value := provider.GitAuthHeader("the-token")

	if key != "http.extraheader" {
		t.Errorf("expected http.extraheader, got %q", key)
	}

	const prefix = "AUTHORIZATION: basic "

	if !strings.HasPrefix(value, prefix) {
		t.Fatalf("expected a basic authorization header, got %q", value)
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		t.Fatalf("the header is not base64: %v", err)
	}

	if string(decoded) != "x-access-token:the-token" {
		t.Errorf("expected the x-access-token credentials, got %q", string(decoded))
	}
}

// The interface name and the default base URL are what the CLI relies on.
func TestNewGitHubDefaults(t *testing.T) {
	provider := NewGitHub("")

	if provider.Name() != "github" {
		t.Errorf("expected github, got %q", provider.Name())
	}

	if provider.baseURL != GitHubDefaultAPI {
		t.Errorf("expected %q, got %q", GitHubDefaultAPI, provider.baseURL)
	}
}

// A name is a path segment pair, and one that could escape its segment is not
// a name.
func TestValidFullName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"owner and name", "owner/repo", true},
		{"a dot in the name", "owner/repo.js", true},
		{"an underscore and a dash", "own_er/re-po", true},
		{"no owner", "/repo", false},
		{"no name", "owner/", false},
		{"a second slash", "owner/repo/extra", false},
		{"a dot-dot owner", "../repo", false},
		{"a dot-dot name", "owner/..", false},
		{"a dot owner", "./repo", false},
		{"a space", "owner/re po", false},
		{"nothing", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidFullName(tt.input); got != tt.want {
				t.Errorf("ValidFullName(%q) = %t, want %t", tt.input, got, tt.want)
			}
		})
	}
}
