package gitprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func fakeBitbucket(t *testing.T, handler http.HandlerFunc) (*Bitbucket, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return NewBitbucket(server.URL), server
}

func TestBitbucketViewer(t *testing.T) {
	var gotAuth string

	provider, _ := fakeBitbucket(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")

		if r.URL.Path != "/user" {
			t.Errorf("expected /user, got %s", r.URL.Path)
		}

		_, _ = w.Write([]byte(`{"username":"alice","display_name":"Alice"}`))
	})

	login, err := provider.Viewer(context.Background(), "the-token")
	if err != nil {
		t.Fatalf("Viewer() error = %v", err)
	}
	if login != "alice" {
		t.Errorf("Viewer() = %q, want alice", login)
	}
	if gotAuth != "Bearer the-token" {
		t.Errorf("Authorization = %q, want the bearer token", gotAuth)
	}
}

// Bitbucket has no "every repository of this user" endpoint: the listing walks
// the workspaces, then pages each one.
func TestBitbucketListRepositoriesWalksWorkspaces(t *testing.T) {
	var paths []string

	provider, _ := fakeBitbucket(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)

		switch {
		case r.URL.Path == "/workspaces":
			_ = json.NewEncoder(w).Encode(bitbucketPage[bitbucketWorkspace]{
				Values: []bitbucketWorkspace{{Slug: "team"}},
			})
		case r.URL.Path == "/repositories/team":
			page := r.URL.Query().Get("page")
			if page == "1" {
				values := make([]bitbucketRepository, 0, bitbucketPageSize)
				for i := 0; i < bitbucketPageSize; i++ {
					values = append(values, bitbucketRepository{FullName: "team/repo"})
				}
				_ = json.NewEncoder(w).Encode(bitbucketPage[bitbucketRepository]{Values: values})
				return
			}

			repository := bitbucketRepository{
				FullName:    "team/last",
				Slug:        "last",
				IsPrivate:   true,
				Description: "the last one",
				MainBranch:  bitbucketBranch{Name: "main"},
			}
			repository.Workspace.Slug = "team"
			repository.Links.Clone = append(repository.Links.Clone,
				bitbucketCloneLink{Name: "https", Href: "https://bitbucket.org/team/last.git"})

			_ = json.NewEncoder(w).Encode(bitbucketPage[bitbucketRepository]{Values: []bitbucketRepository{repository}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	repositories, err := provider.ListRepositories(context.Background(), "the-token")
	if err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}
	if len(repositories) != bitbucketPageSize+1 {
		t.Fatalf("repositories = %d, want %d", len(repositories), bitbucketPageSize+1)
	}

	last := repositories[len(repositories)-1]
	if last.FullName != "team/last" || last.Owner != "team" || !last.Private {
		t.Errorf("last repository = %+v, want a private team/last", last)
	}
	if last.CloneURL != "https://bitbucket.org/team/last.git" || last.DefaultBranch != "main" {
		t.Errorf("last repository = %+v, want the clone URL and branch", last)
	}

	if len(paths) < 3 {
		t.Errorf("paths = %v, want a workspace call and two repository pages", paths)
	}
}

func TestBitbucketGetRepository(t *testing.T) {
	var gotPath string

	provider, _ := fakeBitbucket(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		repository := bitbucketRepository{FullName: "team/project", Slug: "project"}
		_ = json.NewEncoder(w).Encode(repository)
	})

	repository, err := provider.GetRepository(context.Background(), "the-token", "team/project")
	if err != nil {
		t.Fatalf("GetRepository() error = %v", err)
	}
	if repository.FullName != "team/project" {
		t.Errorf("FullName = %q, want team/project", repository.FullName)
	}
	if gotPath != "/repositories/team/project" {
		t.Errorf("path = %q, want /repositories/team/project", gotPath)
	}
}

func TestBitbucketErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, ErrUnauthorized},
		{"not found", http.StatusNotFound, ErrNotFound},
		{"rate limited", http.StatusTooManyRequests, ErrRateLimited},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, _ := fakeBitbucket(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			})

			_, err := provider.Viewer(context.Background(), "the-token")
			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestBitbucketAuthHeader(t *testing.T) {
	key, value := NewBitbucket("").GitAuthHeader("the-token")

	want := "AUTHORIZATION: basic " + base64.StdEncoding.EncodeToString([]byte("x-token-auth:the-token"))
	if key != "http.extraheader" || value != want {
		t.Errorf("header = %q: %q, want http.extraheader: %q", key, value, want)
	}
}

func TestBitbucketDefaultAPI(t *testing.T) {
	if got := NewBitbucket("").baseURL; got != "https://api.bitbucket.org/2.0" {
		t.Errorf("default base = %q", got)
	}
}

// The user lookup falls back to the display name: Bitbucket removed
// `username` for some token types.
func TestBitbucketViewerFallsBackToTheDisplayName(t *testing.T) {
	provider, _ := fakeBitbucket(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"username":"","display_name":"Alice"}`))
	})

	login, err := provider.Viewer(context.Background(), "the-token")
	if err != nil {
		t.Fatalf("Viewer() error = %v", err)
	}
	if login != "Alice" {
		t.Errorf("Viewer() = %q, want the display name", login)
	}
}
