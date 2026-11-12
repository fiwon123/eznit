package apiclient

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := New(server.URL)
	client.Token = "the-token"

	return client
}

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", DefaultBaseURL},
		{"http://localhost:4000", "http://localhost:4000"},
		{"http://localhost:4000/", "http://localhost:4000"},
		{"  http://example.test/api/  ", "http://example.test/api"},
	}

	for _, tt := range tests {
		if got := NormalizeBaseURL(tt.in); got != tt.want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// An address a person typed is checked before it is saved, and the refusal
// says what to fix.
func TestValidateBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{"local", "http://127.0.0.1:4000", "http://127.0.0.1:4000", ""},
		{"https with path", "https://eznit.example.com/api/", "https://eznit.example.com/api", ""},
		{"surrounding space", "  http://example.test  ", "http://example.test", ""},
		{"no scheme", "eznit.example.com", "", "http:// or https://"},
		{"wrong scheme", "ftp://example.test", "", "http:// or https://"},
		{"no host", "http://", "", "needs a host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateBaseURL(tt.in)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ValidateBaseURL(%q) = %q, want an error", tt.in, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to mention %q", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ValidateBaseURL(%q) error = %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ValidateBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// A token belongs to the instance that issued it, so a port change on one host
// is the same instance and a different host is not.
func TestSameHost(t *testing.T) {
	tests := []struct {
		left, right string
		want        bool
	}{
		{"http://127.0.0.1:4000", "http://127.0.0.1:5000", true},
		{"http://localhost:4000", "https://localhost:8443/api", true},
		{"http://127.0.0.1:4000", "http://localhost:4000", false},
		{"http://127.0.0.1:4000", "https://eznit.example.com", false},
		{"", "", true},
	}

	for _, tt := range tests {
		if got := SameHost(tt.left, tt.right); got != tt.want {
			t.Errorf("SameHost(%q, %q) = %v, want %v", tt.left, tt.right, got, tt.want)
		}
	}
}

// Health is what the Files tab's Test button calls: it must not need a
// session, and it must report a server that answers wrongly.
func TestHealth(t *testing.T) {
	t.Run("answering", func(t *testing.T) {
		client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/healthcheck" {
				t.Errorf("path = %q, want the health check", r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "" {
				t.Errorf("Authorization = %q, want none: the health check needs no session", got)
			}

			w.WriteHeader(http.StatusOK)
		})

		if err := client.Health(); err != nil {
			t.Errorf("Health() = %v, want nil", err)
		}
	})

	t.Run("answering with an error", func(t *testing.T) {
		client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})

		err := client.Health()
		if err == nil {
			t.Fatal("Health() = nil, want an error")
		}
		if !strings.Contains(err.Error(), "503") {
			t.Errorf("Health() = %q, want the status", err)
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		address := server.URL
		server.Close()

		if err := New(address).Health(); err == nil {
			t.Error("Health() against a closed server = nil, want an error")
		}
	})
}

func TestLogin(t *testing.T) {
	var got map[string]string

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/login" || r.Method != http.MethodPost {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"status_code":200,"data":{"token":"abc"}}`))
	})

	token, err := client.Login("alice@example.com", "secret")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if token != "abc" {
		t.Errorf("token = %q, want abc", token)
	}
	if got["email"] != "alice@example.com" || got["password"] != "secret" {
		t.Errorf("body = %v, want the credentials", got)
	}
}

func TestLoginRefused(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid credentials"}`))
	})

	_, err := client.Login("alice@example.com", "wrong")

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want an APIError", err)
	}
	if apiErr.Status != http.StatusUnauthorized || apiErr.Message != "invalid credentials" {
		t.Errorf("error = %+v, want 401 + the host message", apiErr)
	}
}

func TestSignupSendsTheConfirmation(t *testing.T) {
	var got map[string]string

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status_code":201}`))
	})

	if err := client.Signup("a@b.c", "pw", "pw"); err != nil {
		t.Fatalf("Signup() error = %v", err)
	}
	if got["confirm_password"] != "pw" {
		t.Errorf("confirm_password = %q, want pw", got["confirm_password"])
	}
}

func TestListFiles(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer the-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":{"data":[{"id":"1","name":"a.txt","ext":"txt","version":2,"is_public":true}],"total":1}}`))
	})

	files, err := client.ListFiles()
	if err != nil {
		t.Fatalf("ListFiles() error = %v", err)
	}
	if len(files) != 1 || files[0].Name != "a.txt" || files[0].Version != 2 || !files[0].IsPublic {
		t.Errorf("files = %+v, want the one file", files)
	}
}

func TestListFilesEmptyIsNotNil(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"data":null,"total":0}}`))
	})

	files, err := client.ListFiles()
	if err != nil {
		t.Fatalf("ListFiles() error = %v", err)
	}
	if files == nil {
		t.Error("files = nil, want an empty slice")
	}
}

func TestUploadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var name, content string

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files" || r.Method != http.MethodPost {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer file.Close()
		raw, _ := io.ReadAll(file)
		name, content = header.Filename, string(raw)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status_code":201}`))
	})

	if err := client.UploadFile(path); err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}
	if name != "notes.txt" || content != "hello" {
		t.Errorf("uploaded %q with %q, want notes.txt with hello", name, content)
	}
}

func TestDownloadFile(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files/abc/content" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Disposition", `attachment; filename="server-name.txt"`)
		_, _ = w.Write([]byte("bytes"))
	})

	dest := filepath.Join(t.TempDir(), "out.txt")
	if _, err := client.DownloadFile("abc", dest); err != nil {
		t.Fatalf("DownloadFile() error = %v", err)
	}

	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(raw) != "bytes" {
		t.Errorf("downloaded %q, want bytes", raw)
	}
}

func TestDownloadFileRefused(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})

	if _, err := client.DownloadFile("abc", filepath.Join(t.TempDir(), "x")); err == nil {
		t.Fatal("DownloadFile() error = nil, want the refusal")
	}
}

func TestDeleteAndShare(t *testing.T) {
	var share map[string]bool

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/files/abc":
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files/abc/share":
			_ = json.NewDecoder(r.Body).Decode(&share)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status_code":200}`))
	})

	if err := client.DeleteFile("abc"); err != nil {
		t.Fatalf("DeleteFile() error = %v", err)
	}
	if err := client.SetFilePublic("abc", true); err != nil {
		t.Fatalf("SetFilePublic() error = %v", err)
	}
	if !share["is_public"] {
		t.Errorf("share body = %v, want is_public true", share)
	}
}

func TestErrorsUseTheEnvelopeMessage(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"already there"}`))
	})

	err := client.DeleteFile("abc")

	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "already there" {
		t.Errorf("error = %v, want an APIError with the message", err)
	}
	if !strings.Contains(err.Error(), "already there") {
		t.Errorf("Error() = %q", err.Error())
	}
}
