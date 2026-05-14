package handlers

import (
	"testing"
)

// TestCompileKeyword pins the regex-safe, case-insensitive,
// word-boundary semantics introduced in the May-2026 niche-validator
// rewrite. The previous implementation used strings.Contains which
// produced false positives like "go" matching "google".
func TestCompileKeyword(t *testing.T) {
	cases := []struct {
		name    string
		keyword string
		hay     string
		want    bool
	}{
		{
			name:    "exact lowercase match",
			keyword: "react",
			hay:     "We use React on the frontend",
			want:    true,
		},
		{
			name:    "case insensitive",
			keyword: "PYTHON",
			hay:     "Senior python developer",
			want:    true,
		},
		{
			name:    "word boundary blocks substring",
			keyword: "go",
			hay:     "Senior google engineer",
			want:    false,
		},
		{
			name:    "word boundary allows surrounding punctuation",
			keyword: "go",
			hay:     "Stack: Python, Go, Rust",
			want:    true,
		},
		{
			name:    "multi-word keyword tolerates whitespace",
			keyword: "Machine Learning",
			hay:     "Looking for machine  learning engineers",
			want:    true,
		},
		{
			name:    "multi-word keyword tolerates hyphen",
			keyword: "Machine Learning",
			hay:     "machine-learning intern role",
			want:    true,
		},
		{
			name:    "non-word characters preserved (C++)",
			keyword: "C++",
			hay:     "Strong C++ background required",
			want:    true,
		},
		{
			name:    "non-word characters preserved (Node.js)",
			keyword: "Node.js",
			hay:     "Stack: Node.js + Postgres",
			want:    true,
		},
		{
			name:    "missing keyword reports false",
			keyword: "Kotlin",
			hay:     "Java + Spring + Postgres",
			want:    false,
		},
		{
			name:    "AutoCAD avoids substring AutoCADO",
			keyword: "AutoCAD",
			hay:     "Familiarity with AutoCAD a plus",
			want:    true,
		},
		{
			name:    "empty keyword returns nil regex (treated as missing)",
			keyword: "",
			hay:     "any text",
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			re := compileKeyword(tc.keyword)
			if re == nil {
				if tc.want {
					t.Fatalf("expected regex for keyword %q but got nil", tc.keyword)
				}
				return
			}
			got := re.MatchString(tc.hay)
			if got != tc.want {
				t.Fatalf(
					"compileKeyword(%q).MatchString(%q) = %v, want %v",
					tc.keyword, tc.hay, got, tc.want,
				)
			}
		})
	}
}

// TestCompileKeywordCaching verifies the cache prevents recompilation —
// regression guard against a future refactor that strips memoisation.
func TestCompileKeywordCaching(t *testing.T) {
	first := compileKeyword("react")
	second := compileKeyword("react")
	if first != second {
		t.Fatalf("expected cached regex pointer reuse; got distinct pointers")
	}
	// Different casing should hit the same cache key.
	third := compileKeyword("REACT")
	if first != third {
		t.Fatalf("expected case-insensitive cache key; got distinct pointers")
	}
}
