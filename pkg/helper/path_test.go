package helper

import "testing"

func TestPathIsInside(t *testing.T) {
	cases := []struct {
		name string
		root string
		path string
		want bool
	}{
		{
			name: "a file in a subfolder is inside",
			root: "/data/uploads",
			path: "/data/uploads/user-id/file-id/1_report.txt",
			want: true,
		},
		{
			name: "the root itself is inside",
			root: "/data/uploads",
			path: "/data/uploads",
			want: true,
		},
		{
			name: "a parent folder is outside",
			root: "/data/uploads",
			path: "/data",
			want: false,
		},
		{
			name: "a sibling with the same prefix is outside",
			root: "/data/uploads",
			path: "/data/uploads-other/secret.txt",
			want: false,
		},
		{
			name: "a traversal is outside",
			root: "/data/uploads",
			path: "/data/uploads/../../etc/passwd",
			want: false,
		},
		{
			name: "an empty root is not inside",
			root: "",
			path: "/data/uploads/file.txt",
			want: false,
		},
		{
			name: "an empty path is not inside",
			root: "/data/uploads",
			path: "",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PathIsInside(tc.root, tc.path); got != tc.want {
				t.Errorf("expected %v for %q in %q, got %v", tc.want, tc.path, tc.root, got)
			}
		})
	}
}

// A relative path inside a root is accepted and comes back canonical.
func TestSafeRelPathAcceptsARelativePath(t *testing.T) {
	got, ok := SafeRelPath(".opencode/agent.md")
	if !ok {
		t.Fatal("expected the relative path to be accepted")
	}

	if got != ".opencode/agent.md" {
		t.Errorf("expected the path unchanged, got %q", got)
	}
}

// A trailing separator is a redundant segment, not a different path: the
// canonical form has no empty segment in it.
func TestSafeRelPathRejectsAnEmptySegment(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"trailing slash", "docs/"},
		{"double slash", "docs//readme.md"},
		{"leading slash", "/docs/readme.md"},
		{"dot segment", "./readme.md"},
		{"parent segment", "../readme.md"},
		{"parent inside", "docs/../../readme.md"},
		{"windows separator", `docs\readme.md`},
		{"absolute windows", `C:\Windows\System32`},
		{"nul byte", "readme.md\x00.txt"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := SafeRelPath(tt.path); ok {
				t.Errorf("expected %q to be refused", tt.path)
			}
		})
	}
}

// A path made only of dots is a directory reference, not a file name.
func TestSafeRelPathRejectsDotOnlyPaths(t *testing.T) {
	for _, path := range []string{".", "..", "./", "../"} {
		if _, ok := SafeRelPath(path); ok {
			t.Errorf("expected %q to be refused", path)
		}
	}
}
