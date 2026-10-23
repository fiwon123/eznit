// Package gitprovider talks to a git host — GitHub first — on behalf of the
// CLI. It holds no credential: the token is an argument to every call and is
// never stored here, so the credential lives only in the caller's config file.
package gitprovider

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

// The ways a provider call can fail. A caller switches on these rather than
// on a message: the message can change, the meaning cannot.
var (
	// ErrUnauthorized means the host refused the token: wrong, expired, or
	// missing a scope.
	ErrUnauthorized = errors.New("the token was refused")
	// ErrNotFound means the repository does not exist for this token —
	// either it is not there, or the token cannot see it. The two are
	// deliberately the same answer: whether a private repository exists is
	// not a caller's to learn.
	ErrNotFound = errors.New("repository not found")
	// ErrRateLimited means the host asked the caller to wait.
	ErrRateLimited = errors.New("the host rate limit was reached")
)

// Repository is one repository a provider exposes.
type Repository struct {
	// FullName is `<owner>/<name>` as the host spells it.
	FullName      string
	Owner         string
	Name          string
	Private       bool
	CloneURL      string
	DefaultBranch string
	Description   string
	UpdatedAt     time.Time
}

// Provider is one git host behind one interface, so a second host is a
// second implementation rather than a second CLI.
type Provider interface {
	// Name is the short name the CLI takes as --provider.
	Name() string
	// Viewer is the account the token belongs to. It is the cheapest call
	// that proves a token works, and it is what `repo auth` reports.
	Viewer(ctx context.Context, token string) (string, error)
	// ListRepositories returns every repository the token can reach.
	ListRepositories(ctx context.Context, token string) ([]Repository, error)
	// GetRepository returns one repository by `<owner>/<name>`.
	GetRepository(ctx context.Context, token string, fullName string) (Repository, error)
	// GitAuthHeader is the git config entry that authenticates a clone. The
	// caller passes it to git through the environment, never through argv
	// and never through the URL, so the token does not end up in the
	// process list or in `.git/config`.
	GitAuthHeader(token string) (key string, value string)
}

// fullNamePattern is the shape of `<owner>/<name>`. It is a label the host
// hands back, but a clone builds a path from it, so a separator trick, a
// dot-dot or an empty part is refused before it reaches the filesystem.
var fullNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// ValidFullName reports whether a repository name may be used to build a path.
func ValidFullName(fullName string) bool {
	if !fullNamePattern.MatchString(fullName) {
		return false
	}

	owner, name, ok := strings.Cut(fullName, "/")
	if !ok || owner == "." || owner == ".." || name == "." || name == ".." {
		return false
	}

	return true
}
