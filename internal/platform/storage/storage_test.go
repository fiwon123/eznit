package storage

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestFilePathBuildsTheVersionedLayout(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-7111-8111-111111111111")
	fileID := uuid.MustParse("22222222-2222-7222-8222-222222222222")

	got := FilePath("/data/uploads", userID, fileID, 3, "report.txt")
	want := filepath.Join("/data/uploads", userID.String(), fileID.String(), "3_report.txt")
	if got != want {
		t.Fatalf("FilePath = %q, want %q", got, want)
	}
}

func TestUserDir(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-7111-8111-111111111111")
	want := filepath.Join("/data/uploads", userID.String())
	if got := UserDir("/data/uploads", userID); got != want {
		t.Fatalf("UserDir = %q, want %q", got, want)
	}
}

func TestTemplateStoragePathIsRelativeWithForwardSlashes(t *testing.T) {
	templateID := uuid.MustParse("33333333-3333-7333-8333-333333333333")
	fileID := uuid.MustParse("44444444-4444-7444-8444-444444444444")

	got := TemplateStoragePath(templateID, fileID, "assets/logo.png")
	want := "templates/" + templateID.String() + "/" + fileID.String() + "/logo.png"
	if got != want {
		t.Fatalf("TemplateStoragePath = %q, want %q", got, want)
	}
	if strings.Contains(got, `\`) {
		t.Fatalf("the stored path must use forward slashes, got %q", got)
	}
}

func TestCommandStoragePath(t *testing.T) {
	commandID := uuid.MustParse("55555555-5555-7555-8555-555555555555")
	got := CommandStoragePath(commandID, "fmt.sh")
	want := "commands/" + commandID.String() + "/fmt.sh"
	if got != want {
		t.Fatalf("CommandStoragePath = %q, want %q", got, want)
	}
}

func TestAbsolutePathResolvesAStoredPath(t *testing.T) {
	got := AbsolutePath("/data/uploads", "templates/tid/fid/logo.png")
	want := filepath.Join("/data/uploads", "templates", "tid", "fid", "logo.png")
	if got != want {
		t.Fatalf("AbsolutePath = %q, want %q", got, want)
	}
}
