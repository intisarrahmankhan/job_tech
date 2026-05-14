// Package store holds in-memory fallbacks that keep the API usable when
// the database is unavailable (e.g. dev machines without a running Postgres).
//
// The fallback is intentionally scoped to user-defined ScrapeTasks: jobs
// already have a mock data path in handlers.ListJobs, but targets are
// purely user-generated so we need somewhere to keep them.
package store

import (
	"sort"
	"sync"
	"time"

	"jobscout/models"

	"github.com/google/uuid"
)

// MemoryTargets is the package-level singleton. Use it via the package
// functions below — no need to instantiate.
var MemoryTargets = newMemoryTargets()

type memoryTargets struct {
	mu    sync.RWMutex
	items map[uuid.UUID]*models.ScrapeTask
}

func newMemoryTargets() *memoryTargets {
	return &memoryTargets{items: make(map[uuid.UUID]*models.ScrapeTask)}
}

// List returns a snapshot sorted by CreatedAt descending.
func (m *memoryTargets) List() []models.ScrapeTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]models.ScrapeTask, 0, len(m.items))
	for _, v := range m.items {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// Create stores a new task. The caller should NOT pre-fill ID/timestamps;
// we manage those here so the in-memory and DB-backed paths look identical
// to callers.
func (m *memoryTargets) Create(t models.ScrapeTask) models.ScrapeTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	stored := t
	m.items[t.ID] = &stored
	return stored
}

// UpdateStatus mutates an existing record's run-state fields in-place.
// Used by the ScraperManager once a target's run completes.
func (m *memoryTargets) UpdateStatus(id uuid.UUID, status models.ScrapeTaskStatus, lastErr string, ranAt *time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.items[id]
	if !ok {
		return
	}
	t.Status = status
	t.LastError = lastErr
	if ranAt != nil {
		t.LastRunAt = ranAt
	}
	t.UpdatedAt = time.Now().UTC()
}

// SetActive flips the IsActive flag (Pause/Resume from the UI). Returns
// false if no row exists for the given id.
func (m *memoryTargets) SetActive(id uuid.UUID, active bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.items[id]
	if !ok {
		return false
	}
	t.IsActive = active
	t.UpdatedAt = time.Now().UTC()
	return true
}

// IncrementResultCount nudges the counter shown on the Targeting Dashboard.
func (m *memoryTargets) IncrementResultCount(id uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.items[id]; ok {
		t.ResultCount++
		t.UpdatedAt = time.Now().UTC()
	}
}

// Get fetches a single target by id, or nil if absent.
func (m *memoryTargets) Get(id uuid.UUID) *models.ScrapeTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.items[id]; ok {
		copy := *t
		return &copy
	}
	return nil
}

// Delete removes a target. Returns true if a row was removed.
func (m *memoryTargets) Delete(id uuid.UUID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[id]; !ok {
		return false
	}
	delete(m.items, id)
	return true
}

// DeleteByNicheAndURL removes every DIRECT_URL task whose value matches
// the supplied URL AND whose NicheID matches. Used to cascade a niche
// source deletion into the mirror ScrapeTask that AddNicheSource created.
// Returns the count removed.
func (m *memoryTargets) DeleteByNicheAndURL(nicheID uuid.UUID, url string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for id, t := range m.items {
		if t.NicheID == nil || *t.NicheID != nicheID {
			continue
		}
		if t.Type != models.TaskTypeDirectURL {
			continue
		}
		if t.Value != url {
			continue
		}
		delete(m.items, id)
		removed++
	}
	return removed
}

// DeleteByNiche removes every task bound to the given niche, regardless
// of type. Used by DeleteNiche so the Health Matrix doesn't keep
// scraping URLs whose owning niche profile is gone.
func (m *memoryTargets) DeleteByNiche(nicheID uuid.UUID) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for id, t := range m.items {
		if t.NicheID == nil || *t.NicheID != nicheID {
			continue
		}
		delete(m.items, id)
		removed++
	}
	return removed
}
