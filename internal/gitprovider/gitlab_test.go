package gitprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeGitLab(t *testing.T, handler http.HandlerFunc) (*GitLab, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return NewGitLab(server.URL), server
}

// The token travels in PRIVATE-TOKEN, not in the URL, and the API path is
// under /api/v4 even though the constructor takes the instance root.
func TestGitLabViewer(t *testing.T) {
	var gotToken, gotAgent string

	provider, _ := fakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		gotAgent = r.Header.Get("User-Agent")

		if r.URL.Path != "/api/v4/user" {
			t.Errorf("expected /api/v4/user, got %s", r.URL.Path)
		}
		if strings.Contains(r.URL.RawQuery, "the-token") {
			t.Errorf("the token is in the URL: %q", r.URL.RawQuery)
		}

		_, _ = w.Write([]byte(`{"username":"alice"}`))
	})

	login, err := provider.Viewer(context.Background(), "the-token")
	if err != nil {
		t.Fatalf("Viewer() error = %v", err)
	}
	if login != "alice" {
		t.Errorf("Viewer() = %q, want alice", login)
	}
	if gotToken != "the-token" {
		t.Errorf("PRIVATE-TOKEN = %q, want the token", gotToken)
	}
	if gotAgent != "eznit" {
		t.Errorf("User-Agent = %q, want eznit", gotAgent)
	}
}

func TestGitLabListRepositoriesReadsEveryPage(t *testing.T) {
	var pages []string

	provider, _ := fakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages = append(pages, page)

		projects := []gitlabProject{}

		if page == "1" {
			for i := 0; i < gitlabPageSize; i++ {
				project := gitlabProject{PathWithNamespace: "group/project", Path: "project", Visibility: "private"}
				project.Namespace.Path = "group"
				projects = append(projects, project)
			}
		} else {
			project := gitlabProject{
				PathWithNamespace: "group/last",
				Path:              "last",
				Name:              "Last",
				Visibility:        "public",
				HTTPURLToRepo:     "https://gitlab.com/group/last.git",
				DefaultBranch:     "main",
			}
			project.Namespace.Path = "group"
			projects = append(projects, project)
		}

		_ = json.NewEncoder(w).Encode(projects)
	})

	repositories, err := provider.ListRepositories(context.Background(), "the-token")
	if err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}
	if len(repositories) != gitlabPageSize+1 {
		t.Fatalf("repositories = %d, want %d", len(repositories), gitlabPageSize+1)
	}
	if len(pages) != 2 || pages[0] != "1" || pages[1] != "2" {
		t.Errorf("pages = %v, want 1 then 2", pages)
	}

	last := repositories[len(repositories)-1]
	if last.FullName != "group/last" || last.Owner != "group" || last.Private {
		t.Errorf("last repository = %+v, want a public group/last", last)
	}
	if last.CloneURL != "https://gitlab.com/group/last.git" || last.DefaultBranch != "main" {
		t.Errorf("last repository = %+v, want the clone URL and branch", last)
	}
}

// A project is addressed by its URL-encoded full path.
func TestGitLabGetRepositoryEncodesThePath(t *testing.T) {
	var gotPath string

	provider, _ := fakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		project := gitlabProject{PathWithNamespace: "group/project", Path: "project", Visibility: "public"}
		project.Namespace.Path = "group"
		_ = json.NewEncoder(w).Encode(project)
	})

	repository, err := provider.GetRepository(context.Background(), "the-token", "group/project")
	if err != nil {
		t.Fatalf("GetRepository() error = %v", err)
	}
	if repository.FullName != "group/project" {
		t.Errorf("FullName = %q, want group/project", repository.FullName)
	}
	if gotPath != "/api/v4/projects/group%2Fproject" {
		t.Errorf("path = %q, want the encoded project path", gotPath)
	}
}

func TestGitLabErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		header http.Header
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, nil, ErrUnauthorized},
		{"not found", http.StatusNotFound, nil, ErrNotFound},
		{"rate limited", http.StatusTooManyRequests, nil, ErrRateLimited},
		{"forbidden with a spent limit", http.StatusForbidden, http.Header{"RateLimit-Remaining": {"0"}}, ErrRateLimited},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, _ := fakeGitLab(t, func(w http.ResponseWriter, r *http.Request) {
				for key, values := range tt.header {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}
				w.WriteHeader(tt.status)
			})

			_, err := provider.Viewer(context.Background(), "the-token")
			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestGitLabAuthHeader(t *testing.T) {
	key, value := NewGitLab("").GitAuthHeader("the-token")

	want := "AUTHORIZATION: basic " + base64.StdEncoding.EncodeToString([]byte("oauth2:the-token"))
	if key != "http.extraheader" || value != want {
		t.Errorf("header = %q: %q, want http.extraheader: %q", key, value, want)
	}
}

func TestGitLabDefaultURL(t *testing.T) {
	if got := NewGitLab("").baseURL; got != "https://gitlab.com/api/v4" {
		t.Errorf("default base = %q", got)
	}
}
