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

// The GitHub defaults. The API address is a constructor argument so a GitHub
// Enterprise instance works, and so a test can point it at a local server.
const (
	GitHubDefaultAPI = "https://api.github.com"

	// pageSize is the largest page GitHub serves; maxPages bounds a listing
	// at 2000 repositories, which is a runaway guard rather than a product
	// limit.
	pageSize = 100
	maxPages = 20

	// maxBodyBytes caps a response. A repository listing is JSON and small;
	// a host that answers with something enormous is not a listing.
	maxBodyBytes = 8 << 20
)

// GitHub is the GitHub provider.
type GitHub struct {
	baseURL string
	client  *http.Client
}

// NewGitHub returns a GitHub provider for the given API base URL.
func NewGitHub(baseURL string) *GitHub {
	if baseURL == "" {
		baseURL = GitHubDefaultAPI
	}

	return &GitHub{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Name is the short name the CLI takes as --provider.
func (g *GitHub) Name() string {
	return "github"
}

// GitAuthHeader is the `http.extraheader` git sends with the clone.
//
// GitHub authenticates a git-over-HTTPS request with HTTP Basic, the username
// `x-access-token` and the token as the password. The header is the same one
// the API uses in a different shape, and it is the shape git understands.
func (g *GitHub) GitAuthHeader(token string) (string, string) {
	credentials := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))

	return "http.extraheader", "AUTHORIZATION: basic " + credentials
}

// viewer is the account a token belongs to.
type viewer struct {
	Login string `json:"login"`
}

// Viewer is the account the token belongs to.
func (g *GitHub) Viewer(ctx context.Context, token string) (string, error) {
	var account viewer

	if err := g.get(ctx, token, "/user", nil, &account); err != nil {
		return "", err
	}

	return account.Login, nil
}

// githubRepository is the wire shape of one entry of `GET /user/repos`. It is
// not the exported type: the host nests the owner, and the exported type is
// what every provider can say.
type githubRepository struct {
	FullName      string    `json:"full_name"`
	Name          string    `json:"name"`
	Private       bool      `json:"private"`
	CloneURL      string    `json:"clone_url"`
	DefaultBranch string    `json:"default_branch"`
	Description   string    `json:"description"`
	UpdatedAt     time.Time `json:"updated_at"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
}

// repository turns the wire shape into the provider shape.
func (r githubRepository) repository() Repository {
	return Repository{
		FullName:      r.FullName,
		Owner:         r.Owner.Login,
		Name:          r.Name,
		Private:       r.Private,
		CloneURL:      r.CloneURL,
		DefaultBranch: r.DefaultBranch,
		Description:   r.Description,
		UpdatedAt:     r.UpdatedAt,
	}
}

// ListRepositories returns every repository the token can reach, page by
// page, until a page comes back short.
func (g *GitHub) ListRepositories(ctx context.Context, token string) ([]Repository, error) {
	repositories := []Repository{}

	for page := 1; page <= maxPages; page++ {
		query := url.Values{}
		query.Set("per_page", strconv.Itoa(pageSize))
		query.Set("page", strconv.Itoa(page))
		query.Set("sort", "full_name")
		query.Set("direction", "asc")

		var wire []githubRepository

		if err := g.get(ctx, token, "/user/repos", query, &wire); err != nil {
			return nil, err
		}

		for _, repository := range wire {
			repositories = append(repositories, repository.repository())
		}

		if len(wire) < pageSize {
			break
		}
	}

	return repositories, nil
}

// GetRepository returns one repository by `<owner>/<name>`.
func (g *GitHub) GetRepository(ctx context.Context, token string, fullName string) (Repository, error) {
	if !ValidFullName(fullName) {
		return Repository{}, ErrNotFound
	}

	owner, name, _ := strings.Cut(fullName, "/")

	var wire githubRepository

	// The path is built from the two validated parts, and each is escaped,
	// so a name that contains a slash cannot escape its segment.
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)

	if err := g.get(ctx, token, path, nil, &wire); err != nil {
		return Repository{}, err
	}

	return wire.repository(), nil
}

// get performs one authenticated GET and decodes the JSON body into out.
//
// The token is set on the request only: it is not part of the URL, it is not
// logged, and it is never written into an error message.
func (g *GitHub) get(ctx context.Context, token string, path string, query url.Values, out any) error {
	endpoint := g.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to build the request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	// GitHub refuses a request without a User-Agent.
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
	case http.StatusForbidden, http.StatusTooManyRequests:
		// A 403 from GitHub is either a rate limit or a permission
		// problem; the remaining counter tells the two apart.
		if resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.StatusCode == http.StatusTooManyRequests {
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

// responseMessage is the host's own message, trimmed to something a terminal
// can show. The body is the server's answer, never the request, so it cannot
// contain the token.
func responseMessage(body []byte) string {
	var payload struct {
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &payload); err == nil && payload.Message != "" {
		return payload.Message
	}

	message := strings.TrimSpace(string(body))

	if runes := []rune(message); len(runes) > 200 {
		message = string(runes[:200]) + "…"
	}

	if message == "" {
		return "no message"
	}

	return message
}
