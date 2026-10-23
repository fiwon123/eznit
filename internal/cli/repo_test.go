package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fiwon123/eznit/pkg/logger"
)

// newTestGlobals is the CLI's globals with a logger that writes to a folder
// the test owns, so a test can read the log back and prove no secret is in it.
func newTestGlobals(t *testing.T) (*Globals, string) {
	t.Helper()

	logs := t.TempDir()

	log, err := logger.NewConsole(logs, true, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	return newGlobals(&API{baseURL: "http://127.0.0.1:0"}, t.TempDir(), log), logs
}

// captureStdout runs fn with stdout redirected, and returns what it printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create a pipe: %v", err)
	}

	os.Stdout = writer

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close the pipe: %v", err)
	}

	os.Stdout = original

	printed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read the output: %v", err)
	}

	return string(printed)
}

// assertNoSecret walks the log folder and fails if any file contains the
// secret. The provider token is never logged, at any level.
func assertNoSecret(t *testing.T, folder string, secret string) {
	t.Helper()

	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatalf("failed to read the log folder: %v", err)
	}

	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(folder, entry.Name()))
		if err != nil {
			t.Fatalf("failed to read the log file: %v", err)
		}

		if strings.Contains(string(raw), secret) {
			t.Errorf("a log file contains the secret")
		}
	}
}

// A token is stored only after the host accepts it, and it is never printed.
func TestRepoAuthStoresAVerifiedToken(t *testing.T) {
	useConfig(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer the-secret-token" {
			t.Errorf("expected the token in the header, got %q", r.Header.Get("Authorization"))
		}

		_, _ = w.Write([]byte(`{"login":"octocat"}`))
	}))
	defer server.Close()

	t.Setenv(githubAPIEnv, server.URL)

	globals, logs := newTestGlobals(t)

	printed := captureStdout(t, func() {
		_ = (&RepoAuthCmd{Provider: "github", Token: "the-secret-token"}).Run(globals)
	})

	stored, err := configValue(providerTokenKey("github"))
	if err != nil || stored != "the-secret-token" {
		t.Fatalf("expected the token to be stored, got %q (%v)", stored, err)
	}

	if strings.Contains(printed, "the-secret-token") {
		t.Error("the token was printed")
	}

	if !strings.Contains(printed, "octocat") {
		t.Errorf("expected the account in the confirmation, got %q", printed)
	}

	assertNoSecret(t, logs, "the-secret-token")
}

// A token the host refuses is not stored: storing it would fail on the next
// command with a less obvious message.
func TestRepoAuthRefusesARefusedToken(t *testing.T) {
	useConfig(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer server.Close()

	t.Setenv(githubAPIEnv, server.URL)

	globals, logs := newTestGlobals(t)

	_ = (&RepoAuthCmd{Provider: "github", Token: "the-wrong-token"}).Run(globals)

	if _, err := configValue(providerTokenKey("github")); err == nil {
		t.Error("expected the refused token not to be stored")
	}

	assertNoSecret(t, logs, "the-wrong-token")
}

// Removing the token removes only the provider key.
func TestRepoAuthRemoveKeepsTheSessionToken(t *testing.T) {
	useConfig(t)

	if err := saveToken("session"); err != nil {
		t.Fatalf("failed to store the session token: %v", err)
	}

	if err := setConfigValue(providerTokenKey("github"), "provider"); err != nil {
		t.Fatalf("failed to store the provider token: %v", err)
	}

	globals, _ := newTestGlobals(t)

	_ = (&RepoAuthCmd{Provider: "github", Remove: true}).Run(globals)

	if _, err := configValue(providerTokenKey("github")); err == nil {
		t.Error("expected the provider token to be gone")
	}

	if token, err := getToken(); err != nil || token != "session" {
		t.Errorf("expected the session token to survive, got %q (%v)", token, err)
	}
}

// The listing is the host's answer, one line per repository.
func TestRepoListPrintsTheRepositories(t *testing.T) {
	useConfig(t)

	if err := setConfigValue(providerTokenKey("github"), "the-secret-token"); err != nil {
		t.Fatalf("failed to store the provider token: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repositories := []map[string]any{
			{"full_name": "owner/public", "name": "public", "private": false, "default_branch": "main", "owner": map[string]string{"login": "owner"}},
			{"full_name": "owner/secret", "name": "secret", "private": true, "default_branch": "dev", "owner": map[string]string{"login": "owner"}},
		}

		_ = json.NewEncoder(w).Encode(repositories)
	}))
	defer server.Close()

	t.Setenv(githubAPIEnv, server.URL)

	globals, logs := newTestGlobals(t)

	printed := captureStdout(t, func() {
		_ = (&RepoListCmd{Provider: "github"}).Run(globals)
	})

	for _, want := range []string{"owner/public", "public", "owner/secret", "private"} {
		if !strings.Contains(printed, want) {
			t.Errorf("expected %q in the listing, got %q", want, printed)
		}
	}

	if strings.Contains(printed, "the-secret-token") {
		t.Error("the token was printed")
	}

	assertNoSecret(t, logs, "the-secret-token")
}

// A listing without a token says what to run, and does not call the host.
func TestRepoListNeedsAToken(t *testing.T) {
	useConfig(t)

	called := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	t.Setenv(githubAPIEnv, server.URL)

	globals, _ := newTestGlobals(t)

	_ = (&RepoListCmd{Provider: "github"}).Run(globals)

	if called {
		t.Error("expected no request without a token")
	}
}

// An unknown host is named, and nothing is stored for it.
func TestProviderForRefusesAnUnknownHost(t *testing.T) {
	if _, err := providerFor("bitbucket"); err == nil {
		t.Fatal("expected the unknown host to be refused")
	}
}

// The command line git receives has the plain URL and no token: a token in
// argv is a token in the process list.
func TestCloneArgsCarryNoToken(t *testing.T) {
	args := cloneArgs("https://example.test/owner/repo.git", "/tmp/target")

	want := []string{"clone", "--", "https://example.test/owner/repo.git", "/tmp/target"}

	if len(args) != len(want) {
		t.Fatalf("expected %v, got %v", want, args)
	}

	for i := range want {
		if args[i] != want[i] {
			t.Errorf("argument %d: expected %q, got %q", i, want[i], args[i])
		}
	}

	for _, arg := range args {
		if strings.Contains(arg, "AUTHORIZATION") || strings.Contains(arg, "the-token") {
			t.Errorf("the argument %q carries the credential", arg)
		}
	}
}

// The token goes in through the environment, and anything the caller already
// had for the same keys is dropped rather than shadowing it.
func TestCloneEnvCarriesTheHeader(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"GIT_CONFIG_COUNT=9",
		"GIT_CONFIG_KEY_0=http.sslVerify",
		"GIT_CONFIG_VALUE_0=false",
		"GIT_TERMINAL_PROMPT=1",
	}

	env := cloneEnv("http.extraheader", "AUTHORIZATION: basic abc", base)

	joined := strings.Join(env, "\n")

	for _, want := range []string{
		"PATH=/usr/bin",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.extraheader",
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic abc",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in the environment, got %v", want, env)
		}
	}

	for _, unwanted := range []string{"GIT_CONFIG_COUNT=9", "http.sslVerify", "GIT_TERMINAL_PROMPT=1"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("expected %q to be dropped, got %v", unwanted, env)
		}
	}
}

// The clone target is under the root, and a folder that already holds
// something is refused rather than mixed with a second tree.
func TestCloneTarget(t *testing.T) {
	root := t.TempDir()
	globals, _ := newTestGlobals(t)

	target, ok := cloneTarget(root, "github", "owner/repo", globals)
	if !ok {
		t.Fatal("expected a target for an empty root")
	}

	want := filepath.Join(root, "github", "owner", "repo")
	if target != want {
		t.Errorf("expected %q, got %q", want, target)
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("failed to create the target: %v", err)
	}

	// An empty folder is still a clone target.
	if _, ok := cloneTarget(root, "github", "owner/repo", globals); !ok {
		t.Error("expected an empty folder to be accepted")
	}

	if err := os.WriteFile(filepath.Join(target, "file"), []byte("x"), 0o600); err != nil {
		t.Fatalf("failed to write a file into the target: %v", err)
	}

	if _, ok := cloneTarget(root, "github", "owner/repo", globals); ok {
		t.Error("expected a non-empty folder to be refused")
	}

	if _, ok := cloneTarget(root, "github", "../escape", globals); ok {
		t.Error("expected a name that is not owner/name to be refused")
	}
}

// The clone root is --dir first, then EZNIT_REPOS, then <home>/eznit.
func TestReposRoot(t *testing.T) {
	globals, _ := newTestGlobals(t)

	if root, ok := reposRoot("/from/flag", globals); !ok || root != "/from/flag" {
		t.Errorf("expected the flag to win, got %q (%t)", root, ok)
	}

	t.Setenv(reposEnv, "/from/env")

	if root, ok := reposRoot("", globals); !ok || root != "/from/env" {
		t.Errorf("expected the environment, got %q (%t)", root, ok)
	}

	t.Setenv(reposEnv, "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("the home folder is not known")
	}

	if root, ok := reposRoot("", globals); !ok || root != filepath.Join(home, "eznit") {
		t.Errorf("expected the default, got %q (%t)", root, ok)
	}
}

// gitClone runs git, and the clone's remote is the plain URL: the credential
// is gone when git exits.
func TestGitCloneLeavesNoCredentialInTheConfig(t *testing.T) {
	if _, err := exec.LookPath(gitBinary); err != nil {
		t.Skip("git is not installed")
	}

	source := t.TempDir()

	runGit(t, source, "init", "-q")
	runGit(t, source, "config", "user.email", "test@example.test")
	runGit(t, source, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(source, "hello.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("failed to write a file: %v", err)
	}

	runGit(t, source, "add", ".")
	runGit(t, source, "commit", "-qm", "first")

	target := filepath.Join(t.TempDir(), "clone")

	_, header := newProviderGitAuth(t)

	if err := gitClone(context.Background(), source, target, "http.extraheader", header, io.Discard, io.Discard); err != nil {
		t.Fatalf("expected the clone to work, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(target, "hello.txt")); err != nil {
		t.Fatalf("expected the file in the clone: %v", err)
	}

	config, err := os.ReadFile(filepath.Join(target, ".git", "config"))
	if err != nil {
		t.Fatalf("failed to read the clone's config: %v", err)
	}

	if strings.Contains(string(config), "AUTHORIZATION") || strings.Contains(string(config), "the-token") {
		t.Errorf("the credential was written into .git/config: %s", config)
	}

	if !strings.Contains(string(config), source) {
		t.Errorf("expected the plain URL as the remote, got %s", config)
	}
}

// newProviderGitAuth is the header the GitHub provider builds for a token.
func newProviderGitAuth(t *testing.T) (string, string) {
	t.Helper()

	provider, err := providerFor("github")
	if err != nil {
		t.Fatalf("failed to build the provider: %v", err)
	}

	return provider.GitAuthHeader("the-token")
}

// runGit runs a git command in dir, and fails the test if it does not exit 0.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	command := exec.Command(gitBinary, args...)
	command.Dir = dir

	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v: %s", args, err, output)
	}
}

// The whole command: the host answers with a clone URL, the clone lands in
// the eznit folder, and nothing that was printed carries the token.
func TestRepoCloneClonesIntoTheReposRoot(t *testing.T) {
	if _, err := exec.LookPath(gitBinary); err != nil {
		t.Skip("git is not installed")
	}

	useConfig(t)

	source := t.TempDir()

	runGit(t, source, "init", "-q")
	runGit(t, source, "config", "user.email", "test@example.test")
	runGit(t, source, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(source, "hello.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("failed to write a file: %v", err)
	}

	runGit(t, source, "add", ".")
	runGit(t, source, "commit", "-qm", "first")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo" {
			t.Errorf("expected /repos/owner/repo, got %s", r.URL.Path)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"full_name":      "owner/repo",
			"name":           "repo",
			"clone_url":      source,
			"default_branch": "main",
			"owner":          map[string]string{"login": "owner"},
		})
	}))
	defer server.Close()

	t.Setenv(githubAPIEnv, server.URL)

	root := t.TempDir()
	t.Setenv(reposEnv, root)

	if err := setConfigValue(providerTokenKey("github"), "the-secret-token"); err != nil {
		t.Fatalf("failed to store the token: %v", err)
	}

	globals, logs := newTestGlobals(t)

	printed := captureStdout(t, func() {
		_ = (&RepoCloneCmd{Name: "owner/repo", Provider: "github"}).Run(globals)
	})

	target := filepath.Join(root, "github", "owner", "repo")

	if _, err := os.Stat(filepath.Join(target, "hello.txt")); err != nil {
		t.Fatalf("expected the clone at %q: %v", target, err)
	}

	if strings.Contains(printed, "the-secret-token") {
		t.Error("the token was printed")
	}

	assertNoSecret(t, logs, "the-secret-token")

	// A second clone into the same folder is refused before git runs.
	again := captureStdout(t, func() {
		_ = (&RepoCloneCmd{Name: "owner/repo", Provider: "github"}).Run(globals)
	})

	if strings.Contains(again, "cloning") {
		t.Errorf("expected the second clone to be refused, got %q", again)
	}
}

// A repository the host does not know is a message, not a panic.
func TestRepoCloneRefusesAnUnknownRepository(t *testing.T) {
	useConfig(t)

	source := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer server.Close()

	t.Setenv(githubAPIEnv, server.URL)
	t.Setenv(reposEnv, source)

	if err := setConfigValue(providerTokenKey("github"), "the-secret-token"); err != nil {
		t.Fatalf("failed to store the token: %v", err)
	}

	globals, _ := newTestGlobals(t)

	printed := captureStdout(t, func() {
		_ = (&RepoCloneCmd{Name: "owner/missing", Provider: "github"}).Run(globals)
	})

	if strings.Contains(printed, "cloning") {
		t.Errorf("expected no clone, got %q", printed)
	}

	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatalf("failed to read the repos root: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected nothing written, found %d entries", len(entries))
	}
}

// A name that is not owner/name is refused before the host is called.
func TestRepoCloneRefusesABadName(t *testing.T) {
	useConfig(t)

	called := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	t.Setenv(githubAPIEnv, server.URL)

	globals, _ := newTestGlobals(t)

	_ = (&RepoCloneCmd{Name: "../../etc/passwd", Provider: "github"}).Run(globals)

	if called {
		t.Error("expected no request for an invalid name")
	}
}
