package scraper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jobscout/config"
)

// TestArgvForTarget pins the exit-2 regression: argv must never lack
// `--url` or `--seed-keywords` when the target is well-formed, and the
// function must FAIL FAST (return an error, never produce a partial
// argv) for malformed targets so the manager can surface a clear
// "value is empty" instead of letting Python crash with exit 2.
func TestArgvForTarget(t *testing.T) {
	type expect struct {
		ok    bool
		flags []string
	}
	cases := []struct {
		name   string
		target *Target
		want   expect
	}{
		{
			name: "direct url with scheme",
			target: &Target{
				ID: "task-1", Type: "DIRECT_URL", Value: "https://example.com/jobs",
			},
			want: expect{ok: true, flags: []string{"--url", "https://example.com/jobs", "--task-id", "task-1", "--detail-pages"}},
		},
		{
			name: "direct url without scheme gets https prepended",
			target: &Target{
				ID: "t2", Type: "DIRECT_URL", Value: "linkedin.com/jobs",
			},
			want: expect{ok: true, flags: []string{"--url", "https://linkedin.com/jobs"}},
		},
		{
			name: "keyword target",
			target: &Target{
				ID: "t3", Type: "KEYWORD", Value: "golang",
			},
			want: expect{ok: true, flags: []string{"--seed-keywords", "golang"}},
		},
		{
			name: "lowercase type still accepted",
			target: &Target{
				ID: "t4", Type: "direct_url", Value: "https://x.com",
			},
			want: expect{ok: true, flags: []string{"--url", "https://x.com"}},
		},
		{
			name: "empty value rejected",
			target: &Target{
				ID: "t5", Type: "DIRECT_URL", Value: "  ",
			},
			want: expect{ok: false},
		},
		{
			name: "unknown type rejected",
			target: &Target{
				ID: "t6", Type: "GARBAGE", Value: "anything",
			},
			want: expect{ok: false},
		},
		{
			name:   "nil target rejected",
			target: nil,
			want:   expect{ok: false},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			argv, err := argvForTarget("/abs/scraper.py", tc.target)
			if tc.want.ok {
				if err != nil {
					t.Fatalf("expected ok, got err: %v", err)
				}
				if len(argv) == 0 || argv[0] != "/abs/scraper.py" {
					t.Fatalf("argv must start with script path; got %v", argv)
				}
				joined := strings.Join(argv, " ")
				for _, f := range tc.want.flags {
					if !strings.Contains(joined, f) {
						t.Errorf("expected %q in argv; got %q", f, joined)
					}
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error, got argv=%v", argv)
			}
			if argv != nil {
				t.Errorf("expected nil argv on error path, got %v", argv)
			}
		})
	}
}

// TestNormaliseURL pins the scheme-prepending behaviour.
func TestNormaliseURL(t *testing.T) {
	cases := map[string]string{
		"https://example.com":   "https://example.com",
		"http://example.com":    "http://example.com",
		"HTTPS://x.io":          "HTTPS://x.io",
		"example.com/jobs":      "https://example.com/jobs",
		"  linkedin.com/jobs ":  "https://linkedin.com/jobs",
		"":                      "",
		"realpython.github.io/": "https://realpython.github.io/",
	}
	for in, want := range cases {
		got := normaliseURL(in)
		if got != want {
			t.Errorf("normaliseURL(%q) = %q; want %q", in, got, want)
		}
	}
}

// TestResolveScript verifies ResolveScript:
//   - returns absolute paths regardless of how the path was supplied
//   - errors when the file doesn't exist (root cause of exit-2)
//   - errors when the path is empty.
func TestResolveScript(t *testing.T) {
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "scraper.py")
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env python\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("absolute path returns absolute path", func(t *testing.T) {
		cfg := &config.Config{ScraperScript: scriptPath}
		abs, dir, err := ResolveScript(cfg)
		if err != nil {
			t.Fatalf("expected ok, got %v", err)
		}
		if !filepath.IsAbs(abs) {
			t.Errorf("expected absolute path; got %q", abs)
		}
		if dir != filepath.Dir(scriptPath) {
			t.Errorf("dir mismatch: got %q want %q", dir, filepath.Dir(scriptPath))
		}
	})

	t.Run("missing file returns clear error", func(t *testing.T) {
		cfg := &config.Config{ScraperScript: filepath.Join(tmp, "nope.py")}
		_, _, err := ResolveScript(cfg)
		if err == nil {
			t.Fatal("expected error for missing file, got nil")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' in error, got %q", err.Error())
		}
	})

	t.Run("empty SCRAPER_SCRIPT rejected", func(t *testing.T) {
		cfg := &config.Config{ScraperScript: ""}
		_, _, err := ResolveScript(cfg)
		if err == nil {
			t.Fatal("expected error for empty path, got nil")
		}
	})
}
