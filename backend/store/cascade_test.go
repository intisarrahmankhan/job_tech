package store

import (
	"testing"

	"jobscout/models"

	"github.com/google/uuid"
)

// TestDeleteByNicheAndURL pins the niche-source cascade behaviour: when
// a NicheSource is removed from /niches the mirror ScrapeTask created
// by AddNicheSource must also disappear so the Health Matrix stops
// listing the URL as an active pipeline.
func TestDeleteByNicheAndURL(t *testing.T) {
	store := newMemoryTargets()
	nicheA := uuid.New()
	nicheB := uuid.New()

	url := "https://example.com/jobs"

	store.Create(models.ScrapeTask{Type: models.TaskTypeDirectURL, Value: url, NicheID: &nicheA, IsActive: true})
	store.Create(models.ScrapeTask{Type: models.TaskTypeDirectURL, Value: url, NicheID: &nicheB, IsActive: true})        // different niche — must survive
	store.Create(models.ScrapeTask{Type: models.TaskTypeDirectURL, Value: "https://other.com", NicheID: &nicheA, IsActive: true}) // different URL — must survive
	store.Create(models.ScrapeTask{Type: models.TaskTypeKeyword, Value: url, NicheID: &nicheA, IsActive: true})         // not DIRECT_URL — must survive

	removed := store.DeleteByNicheAndURL(nicheA, url)
	if removed != 1 {
		t.Errorf("expected 1 row deleted, got %d", removed)
	}
	if got := len(store.List()); got != 3 {
		t.Errorf("expected 3 remaining tasks, got %d", got)
	}
}

// TestDeleteByNiche removes every task bound to a niche regardless of
// type — used by DeleteNiche so killing a niche cleans its scrapers.
func TestDeleteByNiche(t *testing.T) {
	store := newMemoryTargets()
	nicheA := uuid.New()
	nicheB := uuid.New()

	store.Create(models.ScrapeTask{Type: models.TaskTypeDirectURL, Value: "https://a.com", NicheID: &nicheA})
	store.Create(models.ScrapeTask{Type: models.TaskTypeKeyword, Value: "kw", NicheID: &nicheA})
	store.Create(models.ScrapeTask{Type: models.TaskTypeDirectURL, Value: "https://b.com", NicheID: &nicheB})
	store.Create(models.ScrapeTask{Type: models.TaskTypeDirectURL, Value: "https://unbound.com"})

	removed := store.DeleteByNiche(nicheA)
	if removed != 2 {
		t.Errorf("expected 2 deleted, got %d", removed)
	}
	if got := len(store.List()); got != 2 {
		t.Errorf("expected 2 surviving tasks, got %d", got)
	}
}

// TestGetSourceSnapshot ensures GetSource returns a *copy*, not a
// pointer into the map, so callers can't mutate stored state by
// accident.
func TestGetSourceSnapshot(t *testing.T) {
	store := newMemoryNiches()
	niche := uuid.New()
	stored := store.CreateSource(models.NicheSource{NicheID: niche, URL: "https://x.com", Label: "ORIGINAL"})

	got := store.GetSource(stored.ID)
	if got == nil {
		t.Fatal("expected source, got nil")
	}
	got.Label = "MUTATED"

	again := store.GetSource(stored.ID)
	if again.Label != "ORIGINAL" {
		t.Errorf("GetSource returned a shared reference; mutating the copy changed stored data (got %q)", again.Label)
	}
}
