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

// The Bitbucket Cloud defaults.
const (
	BitbucketDefaultAPI = "https://api.bitbucket.org/2.0"

	// Bitbucket caps a page at 100. The bounds keep a runaway listing from
	// turning into an unbounded walk of workspaces.
	bitbucketPageSize  = 100
	bitbucketMaxPages  = 20
	bitbucketMaxSpaces = 20
)

// Bitbucket is the Bitbucket Cloud provider.
type Bitbucket struct {
	baseURL string
	client  *http.Client
}

// NewBitbucket returns a Bitbucket provider for an API base URL.
func NewBitbucket(baseURL string) *Bitbucket {
	if baseURL == "" {
		baseURL = BitbucketDefaultAPI
	}

	return &Bitbucket{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Name is the short name the CLI takes as --provider.
func (b *Bitbucket) Name() string {
	return "bitbucket"
}

// GitAuthHeader is the `http.extraheader` git sends with the clone.
//
// Bitbucket Cloud accepts an access token as the password with the literal
// username `x-token-auth` on a git-over-HTTPS request.
func (b *Bitbucket) GitAuthHeader(token string) (string, string) {
	credentials := base64.StdEncoding.EncodeToString([]byte("x-token-auth:" + token))

	return "http.extraheader", "AUTHORIZATION: basic " + credentials
}

// bitbucketUser is the account a token belongs to. Bitbucket removed
// `username` for some token types, so the display name is the fallback.
type bitbucketUser struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

// Viewer is the account the token belongs to.
func (b *Bitbucket) Viewer(ctx context.Context, token string) (string, error) {
	var account bitbucketUser

	if err := b.get(ctx, token, "/user", nil, &account); err != nil {
		return "", err
	}

	if account.Username != "" {
		return account.Username, nil
	}

	return account.DisplayName, nil
}

// bitbucketWorkspace is one workspace the token can see.
type bitbucketWorkspace struct {
	Slug string `json:"slug"`
}

// bitbucketPage is the shape every Bitbucket listing page shares.
type bitbucketPage[T any] struct {
	Values []T `json:"values"`
}

// bitbucketBranch is the branch named in a repository's `mainbranch`.
type bitbucketBranch struct {
	Name string `json:"name"`
}

// bitbucketCloneLink is one entry of a repository's `links.clone`: the URL and
// the protocol it speaks ("https" or "ssh").
type bitbucketCloneLink struct {
	Name string `json:"name"`
	Href string `json:"href"`
}

// bitbucketRepository is the wire shape of one repository.
type bitbucketRepository struct {
	FullName    string          `json:"full_name"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	IsPrivate   bool            `json:"is_private"`
	Description string          `json:"description"`
	UpdatedOn   time.Time       `json:"updated_on"`
	MainBranch  bitbucketBranch `json:"mainbranch"`
	Workspace   struct {
		Slug string `json:"slug"`
	} `json:"workspace"`
	Links struct {
		Clone []bitbucketCloneLink `json:"clone"`
	} `json:"links"`
}

// repository turns the wire shape into the provider shape.
func (r bitbucketRepository) repository() Repository {
	owner := r.Workspace.Slug
	if owner == "" {
		owner, _, _ = strings.Cut(r.FullName, "/")
	}

	cloneURL := ""
	for _, clone := range r.Links.Clone {
		if clone.Name == "https" {
			cloneURL = clone.Href
			break
		}
		if cloneURL == "" {
			cloneURL = clone.Href
		}
	}

	return Repository{
		FullName:      r.FullName,
		Owner:         owner,
		Name:          r.Slug,
		Private:       r.IsPrivate,
		CloneURL:      cloneURL,
		DefaultBranch: r.MainBranch.Name,
		Description:   r.Description,
		UpdatedAt:     r.UpdatedOn,
	}
}

// ListRepositories walks the token's workspaces and returns the repositories
// of each. Bitbucket has no "every repository of this user" endpoint, so the
// walk is the listing.
func (b *Bitbucket) ListRepositories(ctx context.Context, token string) ([]Repository, error) {
	workspaces, err := b.listWorkspaces(ctx, token)
	if err != nil {
		return nil, err
	}

	repositories := []Repository{}

	for _, workspace := range workspaces {
		page, err := b.listWorkspaceRepositories(ctx, token, workspace.Slug)
		if err != nil {
			return nil, err
		}

		repositories = append(repositories, page...)
	}

	return repositories, nil
}

func (b *Bitbucket) listWorkspaces(ctx context.Context, token string) ([]bitbucketWorkspace, error) {
	var workspaces []bitbucketWorkspace

	for page := 1; page <= bitbucketMaxPages && len(workspaces) < bitbucketMaxSpaces; page++ {
		query := url.Values{}
		query.Set("pagelen", strconv.Itoa(bitbucketPageSize))
		query.Set("page", strconv.Itoa(page))

		var wire bitbucketPage[bitbucketWorkspace]

		if err := b.get(ctx, token, "/workspaces", query, &wire); err != nil {
			return nil, err
		}

		workspaces = append(workspaces, wire.Values...)

		if len(wire.Values) < bitbucketPageSize {
			break
		}
	}

	return workspaces, nil
}

func (b *Bitbucket) listWorkspaceRepositories(ctx context.Context, token string, workspace string) ([]Repository, error) {
	repositories := []Repository{}

	for page := 1; page <= bitbucketMaxPages; page++ {
		query := url.Values{}
		query.Set("pagelen", strconv.Itoa(bitbucketPageSize))
		query.Set("page", strconv.Itoa(page))

		var wire bitbucketPage[bitbucketRepository]

		path := "/repositories/" + url.PathEscape(workspace)
		if err := b.get(ctx, token, path, query, &wire); err != nil {
			return nil, err
		}

		for _, repository := range wire.Values {
			repositories = append(repositories, repository.repository())
		}

		if len(wire.Values) < bitbucketPageSize {
			break
		}
	}

	return repositories, nil
}

// GetRepository returns one repository by `<workspace>/<slug>`.
func (b *Bitbucket) GetRepository(ctx context.Context, token string, fullName string) (Repository, error) {
	if !ValidFullName(fullName) {
		return Repository{}, ErrNotFound
	}

	owner, name, _ := strings.Cut(fullName, "/")
	path := "/repositories/" + url.PathEscape(owner) + "/" + url.PathEscape(name)

	var wire bitbucketRepository

	if err := b.get(ctx, token, path, nil, &wire); err != nil {
		return Repository{}, err
	}

	return wire.repository(), nil
}

// get performs one authenticated GET and decodes the JSON body into out.
func (b *Bitbucket) get(ctx context.Context, token string, path string, query url.Values, out any) error {
	endpoint := b.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to build the request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "eznit")

	resp, err := b.client.Do(req)
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
