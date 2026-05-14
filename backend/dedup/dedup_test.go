package dedup

import "testing"

func TestSourceHashIsDeterministic(t *testing.T) {
	a := SourceHash("Pathao", "Senior Flutter Developer")
	b := SourceHash("  pathao ", "  senior flutter developer ")
	if a != b {
		t.Fatalf("expected deterministic hash, got %s vs %s", a, b)
	}
}

func TestSourceHashCollidesAcrossURLs(t *testing.T) {
	// Same role on two boards must collide so the merge logic fires.
	a := SourceHash("Pathao", "Senior Flutter Developer")
	b := SourceHash("Pathao", "Senior Flutter Developer")
	if a != b {
		t.Fatal("expected same hash for same (company, title)")
	}
}

func TestSourceHashDiffersByTitle(t *testing.T) {
	a := SourceHash("Pathao", "Backend Engineer")
	b := SourceHash("Pathao", "Frontend Engineer")
	if a == b {
		t.Fatal("hashes must differ across titles")
	}
}
