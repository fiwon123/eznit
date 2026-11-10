// Package apiclient is the Go client the desktop app speaks to an eznit API
// with: the same /v1 routes the CLI and the web UI use.
//
// It is deliberately small and typed: the GUI renders these values, so a
// field that is missing here is a field the window cannot show. The CLI keeps
// its own command-shaped requests for now; the two can converge on this
// package without changing the API.
package apiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fiwon123/eznit/pkg/types"
)

// DefaultBaseURL is where the product stack answers when nothing is
// configured.
const DefaultBaseURL = "http://localhost:4000"

// Client talks to one instance.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// File is one row of the caller's library.
type File struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Ext      string `json:"ext"`
	Version  int    `json:"version"`
	IsPublic bool   `json:"is_public"`
}

// APIError is a non-2xx answer, with the host's own message when it sent one.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("the API answered %d", e.Status)
	}

	return e.Message
}

// New returns a client for an instance. An empty base is the local stack.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: NormalizeBaseURL(baseURL),
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// NormalizeBaseURL trims the trailing slash and fills the default.
func NormalizeBaseURL(baseURL string) string {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		return DefaultBaseURL
	}

	return strings.TrimSuffix(base, "/")
}

type listResponse struct {
	Data  []File `json:"data"`
	Total int    `json:"total"`
}

// Login stores nothing: it returns the token for the caller to persist.
func (c *Client) Login(email string, password string) (string, error) {
	body := map[string]string{"email": email, "password": password}

	var envelope types.Envelope[struct {
		Token string `json:"token"`
	}]

	if err := c.do(http.MethodPost, "/v1/login", jsonBody(body), &envelope); err != nil {
		return "", err
	}

	if envelope.Data.Token == "" {
		return "", fmt.Errorf("the API answered without a token")
	}

	return envelope.Data.Token, nil
}

// Signup creates an account. It answers nothing to keep: the caller logs in.
func (c *Client) Signup(email string, password string, confirm string) error {
	body := map[string]string{
		"email":            email,
		"password":         password,
		"confirm_password": confirm,
	}

	var envelope types.Envelope[any]

	return c.do(http.MethodPost, "/v1/signup", jsonBody(body), &envelope)
}

// ListFiles returns the caller's files.
func (c *Client) ListFiles() ([]File, error) {
	var envelope types.Envelope[listResponse]

	if err := c.do(http.MethodGet, "/v1/files", nil, &envelope); err != nil {
		return nil, err
	}

	if envelope.Data.Data == nil {
		return []File{}, nil
	}

	return envelope.Data.Data, nil
}

// UploadFile sends one file from disk and replaces nothing: the API keeps a
// new version when the name already exists.
func (c *Client) UploadFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return fmt.Errorf("build the upload: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish the upload: %w", err)
	}

	request, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/files", &body)
	if err != nil {
		return fmt.Errorf("build the request: %w", err)
	}

	request.Header.Set("Content-Type", writer.FormDataContentType())
	c.authorize(request)

	return c.send(request, nil)
}

// DownloadFile writes the file's bytes to dest.
func (c *Client) DownloadFile(id string, dest string) (string, error) {
	request, err := http.NewRequest(http.MethodGet,
		c.BaseURL+"/v1/files/"+url.PathEscape(id)+"/content", nil)
	if err != nil {
		return "", fmt.Errorf("build the request: %w", err)
	}

	c.authorize(request)

	response, err := c.HTTP.Do(request)
	if err != nil {
		return "", fmt.Errorf("the API could not be reached: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", decodeError(response)
	}

	// The server may suggest a name; the caller chose the destination, so the
	// suggestion is only reported, never used to move the file.
	name := ""
	if disposition := response.Header.Get("Content-Disposition"); disposition != "" {
		if _, params, err := mime.ParseMediaType(disposition); err == nil {
			name = params["filename"]
		}
	}

	target := dest
	if target == "" {
		target = name
	}

	if target == "" {
		return "", fmt.Errorf("no destination was given")
	}

	out, err := os.Create(target)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", target, err)
	}

	if _, err := io.Copy(out, response.Body); err != nil {
		out.Close()
		return "", fmt.Errorf("write %s: %w", target, err)
	}

	if err := out.Close(); err != nil {
		return "", fmt.Errorf("close %s: %w", target, err)
	}

	return target, nil
}

// DeleteFile removes one of the caller's files.
func (c *Client) DeleteFile(id string) error {
	request, err := http.NewRequest(http.MethodDelete,
		c.BaseURL+"/v1/files/"+url.PathEscape(id), nil)
	if err != nil {
		return fmt.Errorf("build the request: %w", err)
	}

	c.authorize(request)

	return c.send(request, nil)
}

// SetFilePublic shares or unshares a file.
func (c *Client) SetFilePublic(id string, public bool) error {
	request, err := http.NewRequest(http.MethodPost,
		c.BaseURL+"/v1/files/"+url.PathEscape(id)+"/share",
		jsonBody(map[string]bool{"is_public": public}))
	if err != nil {
		return fmt.Errorf("build the request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	c.authorize(request)

	return c.send(request, nil)
}

func (c *Client) authorize(request *http.Request) {
	if c.Token != "" {
		request.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

func (c *Client) do(method string, path string, body io.Reader, out any) error {
	request, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return fmt.Errorf("build the request: %w", err)
	}

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	c.authorize(request)

	return c.send(request, out)
}

func (c *Client) send(request *http.Request, out any) error {
	response, err := c.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("the API could not be reached: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return decodeError(response)
	}

	if out == nil {
		return nil
	}

	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return fmt.Errorf("the API answered with something that is not the expected JSON: %w", err)
	}

	return nil
}

// decodeError turns a failed answer into an APIError carrying the host's
// message. The body is the host's answer, never the request, so it cannot
// contain the token.
func decodeError(response *http.Response) error {
	var envelope types.Envelope[any]

	message := ""
	if err := json.NewDecoder(response.Body).Decode(&envelope); err == nil {
		message = envelope.Error
		if message == "" {
			message = envelope.Msg
		}
	}

	return &APIError{Status: response.StatusCode, Message: message}
}

func jsonBody(value any) io.Reader {
	raw, err := json.Marshal(value)
	if err != nil {
		// The values are built here, from strings and bools; a marshal failure
		// is a programming error, and an empty body makes the API answer 400.
		return strings.NewReader("{}")
	}

	return bytes.NewReader(raw)
}
