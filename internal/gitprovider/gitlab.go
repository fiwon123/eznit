package gitprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The GitLab defaults. The base is the instance root, not the API path, so a
// self-hosted instance is the same constructor argument; `/api/v4` is added
// here.
const (
	GitLabDefaultURL = "https://gitlab.com"

	// GitLab caps a page at 100. maxPages bounds a listing at 2000
	// repositories, like the GitHub provider.
	gitlabPageSize = 100
	gitlabMaxPages = 20
)

// GitLab is the GitLab provider.
type GitLab struct {
	baseURL string
	client  *http.Client
}

// NewGitLab returns a GitLab provider for an instance root. An empty URL is
// gitlab.com.
func NewGitLab(instanceURL string) *GitLab {
	if instanceURL == "" {
		instanceURL = GitLabDefaultURL
	}

	return &GitLab{
		baseURL: strings.TrimSuffix(instanceURL, "/") + "/api/v4",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Name is the short name the CLI takes as --provider.
func (g *GitLab) Name() string {
	return "gitlab"
}

// GitAuthHeader is the `http.extraheader` git sends with the clone.
//
// GitLab answers a git-over-HTTPS request with HTTP Basic, the username
// `oauth2` and the personal access token as the password.
func (g *GitLab) GitAuthHeader(token string) (string, string) {
	credentials := base64.StdEncoding.EncodeToString([]byte("oauth2:" + token))

	return "http.extraheader", "AUTHORIZATION: basic " + credentials
}

// gitlabUser is the account a token belongs to.
type gitlabUser struct {
	Username string `json:"username"`
}

// Viewer is the account the token belongs to.
func (g *GitLab) Viewer(ctx context.Context, token string) (string, error) {
	var account gitlabUser

	if err := g.get(ctx, token, "/user", nil, &account); err != nil {
		return "", err
	}

	return account.Username, nil
}

// gitlabProject is the wire shape of one project.
type gitlabProject struct {
	PathWithNamespace string    `json:"path_with_namespace"`
	Name              string    `json:"name"`
	Path              string    `json:"path"`
	Visibility        string    `json:"visibility"`
	HTTPURLToRepo     string    `json:"http_url_to_repo"`
	DefaultBranch     string    `json:"default_branch"`
	Description       string    `json:"description"`
	LastActivityAt    time.Time `json:"last_activity_at"`
	Namespace         struct {
		Path string `json:"path"`
	} `json:"namespace"`
}

// repository turns the wire shape into the provider shape.
func (p gitlabProject) repository() Repository {
	return Repository{
		FullName:      p.PathWithNamespace,
		Owner:         p.Namespace.Path,
		Name:          p.Path,
		Private:       p.Visibility != "public",
		CloneURL:      p.HTTPURLToRepo,
		DefaultBranch: p.DefaultBranch,
		Description:   p.Description,
		UpdatedAt:     p.LastActivityAt,
	}
}

// ListRepositories returns every project the token is a member of, page by
// page, until a page comes back short.
func (g *GitLab) ListRepositories(ctx context.Context, token string) ([]Repository, error) {
	repositories := []Repository{}

	for page := 1; page <= gitlabMaxPages; page++ {
		query := url.Values{}
		query.Set("membership", "true")
		query.Set("per_page", strconv.Itoa(gitlabPageSize))
		query.Set("page", strconv.Itoa(page))
		query.Set("order_by", "path")
		query.Set("sort", "asc")

		var wire []gitlabProject

		if err := g.get(ctx, token, "/projects", query, &wire); err != nil {
			return nil, err
		}

		for _, project := range wire {
			repositories = append(repositories, project.repository())
		}

		if len(wire) < gitlabPageSize {
			break
		}
	}

	return repositories, nil
}

// GetRepository returns one project by `<namespace>/<name>`.
func (g *GitLab) GetRepository(ctx context.Context, token string, fullName string) (Repository, error) {
	if !ValidFullName(fullName) {
		return Repository{}, ErrNotFound
	}

	// GitLab addresses a project by its full path, URL-encoded, slash and
	// all; PathEscape escapes the slash, which is what the API expects.
	path := "/projects/" + url.PathEscape(fullName)

	var wire gitlabProject

	if err := g.get(ctx, token, path, nil, &wire); err != nil {
		return Repository{}, err
	}

	return wire.repository(), nil
}

// get performs one authenticated GET and decodes the JSON body into out.
func (g *GitLab) get(ctx context.Context, token string, path string, query url.Values, out any) error {
	endpoint := g.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to build the request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("User-Agent", "eznit")

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("the host could not be reached: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("failed to read the response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusTooManyRequests:
		return ErrRateLimited
	case http.StatusForbidden:
		// GitLab answers a spent rate limit with 403 on some endpoints and
		// names the remaining count in a header.
		if resp.Header.Get("RateLimit-Remaining") == "0" || resp.Header.Get("RateLimit-Reset") != "" {
			return ErrRateLimited
		}

		return fmt.Errorf("the host refused the request: %s", responseMessage(body))
	default:
		return fmt.Errorf("the host answered %d: %s", resp.StatusCode, responseMessage(body))
	}

	if out == nil {
		return nil
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("the host answered with something that is not the expected JSON: %w", err)
	}

	return nil
}
