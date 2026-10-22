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
