package router

import "testing"

func TestMatchPathPattern_Unit(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		{
			name:    "exact match",
			pattern: "//storage.googleapis.com/projects/my-project/buckets/my-bucket",
			value:   "//storage.googleapis.com/projects/my-project/buckets/my-bucket",
			want:    true,
		},
		{
			name:    "single wildcard matches one segment",
			pattern: "//storage.googleapis.com/projects/_/buckets/*/objects/*",
			value:   "//storage.googleapis.com/projects/_/buckets/my-bucket/objects/my-obj",
			want:    true,
		},
		{
			name:    "single wildcard does not match multiple segments",
			pattern: "//storage.googleapis.com/projects/_/buckets/*/objects/*",
			value:   "//storage.googleapis.com/projects/_/buckets/my-bucket/objects/foo/bar",
			want:    false,
		},
		{
			name:    "double wildcard matches multiple segments",
			pattern: "//storage.googleapis.com/projects/**",
			value:   "//storage.googleapis.com/projects/foo/bar/baz",
			want:    true,
		},
		{
			name:    "double wildcard matches zero extra segments (trailing slash)",
			pattern: "//storage.googleapis.com/projects/**",
			value:   "//storage.googleapis.com/projects/",
			want:    true,
		},
		{
			name:    "double wildcard at start matches any path",
			pattern: "**",
			value:   "anything/here",
			want:    true,
		},
		{
			name:    "double wildcard at start matches empty value",
			pattern: "**",
			value:   "",
			want:    true,
		},
		{
			name:    "double wildcard in middle matches zero segments",
			pattern: "a/**/b",
			value:   "a/b",
			want:    true,
		},
		{
			name:    "double wildcard in middle matches one segment",
			pattern: "a/**/b",
			value:   "a/x/b",
			want:    true,
		},
		{
			name:    "double wildcard in middle matches multiple segments",
			pattern: "a/**/b",
			value:   "a/x/y/z/b",
			want:    true,
		},
		{
			name:    "mismatch: different prefix",
			pattern: "//storage.googleapis.com/projects/**",
			value:   "//other.googleapis.com/projects/foo",
			want:    false,
		},
		{
			name:    "empty pattern matches empty value",
			pattern: "",
			value:   "",
			want:    true,
		},
		{
			name:    "empty pattern does not match non-empty value",
			pattern: "",
			value:   "something",
			want:    false,
		},
		{
			name:    "star-star as substring is treated as literal (not wildcard)",
			pattern: "foo**bar",
			value:   "fooxbar",
			want:    false,
		},
		{
			name:    "star-star as substring literal match",
			pattern: "foo**bar",
			value:   "foo**bar",
			want:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchPathPattern(tc.pattern, tc.value)
			if got != tc.want {
				t.Errorf("matchPathPattern(%q, %q) = %v, want %v", tc.pattern, tc.value, got, tc.want)
			}
		})
	}
}

func TestMatchSegs_Unit(t *testing.T) {
	tests := []struct {
		name  string
		pSegs []string
		vSegs []string
		want  bool
	}{
		{
			name:  "both empty",
			pSegs: []string{},
			vSegs: []string{},
			want:  true,
		},
		{
			name:  "exact single segment",
			pSegs: []string{"foo"},
			vSegs: []string{"foo"},
			want:  true,
		},
		{
			name:  "exact mismatch",
			pSegs: []string{"foo"},
			vSegs: []string{"bar"},
			want:  false,
		},
		{
			name:  "single wildcard matches any segment",
			pSegs: []string{"*"},
			vSegs: []string{"anything"},
			want:  true,
		},
		{
			name:  "single wildcard does not consume multiple segments",
			pSegs: []string{"*"},
			vSegs: []string{"a", "b"},
			want:  false,
		},
		{
			name:  "double wildcard matches zero segments",
			pSegs: []string{"**"},
			vSegs: []string{},
			want:  true,
		},
		{
			name:  "double wildcard matches one segment",
			pSegs: []string{"**"},
			vSegs: []string{"x"},
			want:  true,
		},
		{
			name:  "double wildcard matches multiple segments",
			pSegs: []string{"**"},
			vSegs: []string{"x", "y", "z"},
			want:  true,
		},
		{
			name:  "double wildcard then literal: zero consumed",
			pSegs: []string{"**", "end"},
			vSegs: []string{"end"},
			want:  true,
		},
		{
			name:  "double wildcard then literal: one consumed",
			pSegs: []string{"**", "end"},
			vSegs: []string{"mid", "end"},
			want:  true,
		},
		{
			name:  "pattern longer than value",
			pSegs: []string{"a", "b", "c"},
			vSegs: []string{"a", "b"},
			want:  false,
		},
		{
			name:  "value longer than pattern",
			pSegs: []string{"a", "b"},
			vSegs: []string{"a", "b", "c"},
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchSegs(tc.pSegs, tc.vSegs)
			if got != tc.want {
				t.Errorf("matchSegs(%v, %v) = %v, want %v", tc.pSegs, tc.vSegs, got, tc.want)
			}
		})
	}
}
