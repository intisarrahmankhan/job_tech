package store

import (
	"sort"
	"strings"
	"sync"
	"time"

	"jobscout/models"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// MemoryJobs is the package-level singleton for the in-memory job buffer
// used when Postgres isn't reachable. It supports the merge semantics the
// DB-backed path provides: jobs are keyed by sourceHash, and re-ingesting
// a hash appends new sources/URLs and bumps merge_count instead of
// duplicating the row.
var MemoryJobs = newMemoryJobs()

type memoryJobs struct {
	mu      sync.RWMutex
	byHash  map[string]*models.Job // primary index for merge lookups
	ordered []uuid.UUID            // insertion order, newest last
}

func newMemoryJobs() *memoryJobs {
	return &memoryJobs{byHash: make(map[string]*models.Job)}
}

// Upsert inserts a new Job or merges into an existing one keyed by
// sourceHash. Returns (job, merged) where `merged` is true when an
// existing record was updated rather than a new row created.
func (m *memoryJobs) Upsert(in models.Job) (models.Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.byHash[in.SourceHash]; ok {
		merged := false
		// Add any new source not already present.
		for _, s := range in.Sources {
			if !contains(existing.Sources, s) {
				existing.Sources = append(existing.Sources, s)
				merged = true
			}
		}
		// Same for posting URLs.
		for _, u := range in.MergedURLs {
			if u != "" && !contains(existing.MergedURLs, u) {
				existing.MergedURLs = append(existing.MergedURLs, u)
				merged = true
			}
		}
		if merged {
			existing.MergeCount++
		}
		now := time.Now().UTC()
		existing.LastScrapedAt = &now
		existing.UpdatedAt = now
		return *existing, true
	}

	// Brand new record.
	if in.ID == uuid.Nil {
		in.ID = uuid.New()
	}
	now := time.Now().UTC()
	if in.PostedAt.IsZero() {
		in.PostedAt = now
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	in.LastScrapedAt = &now
	in.IsActive = true
	if in.Sources == nil {
		in.Sources = pq.StringArray{}
	}
	if in.Tags == nil {
		in.Tags = pq.StringArray{}
	}
	if in.MergedURLs == nil {
		in.MergedURLs = pq.StringArray{}
	}
	stored := in
	m.byHash[in.SourceHash] = &stored
	m.ordered = append(m.ordered, in.ID)
	return stored, false
}

// List returns every active job, newest first, optionally filtered by
// the same q/role/location/nicheId semantics the DB path supports.
func (m *memoryJobs) List(q, role, location, nicheID string) []models.Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	all := make([]*models.Job, 0, len(m.byHash))
	for _, j := range m.byHash {
		if !j.IsActive {
			continue
		}
		all = append(all, j)
	}
	sort.Slice(all, func(i, k int) bool {
		return all[i].PostedAt.After(all[k].PostedAt)
	})

	ql := strings.ToLower(strings.TrimSpace(q))
	rl := strings.ToLower(strings.TrimSpace(role))
	ll := strings.ToLower(strings.TrimSpace(location))
	nl := strings.TrimSpace(nicheID)
	out := make([]models.Job, 0, len(all))
	for _, j := range all {
		if nl != "" {
			if j.NicheID == nil || j.NicheID.String() != nl {
				continue
			}
		}
		if ql != "" {
			hay := strings.ToLower(j.Title + " " + j.Company)
			matched := strings.Contains(hay, ql)
			if !matched {
				for _, t := range j.Tags {
					if strings.Contains(strings.ToLower(t), ql) {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}
		}
		if rl != "" && !strings.Contains(strings.ToLower(j.Title), rl) {
			continue
		}
		if ll != "" && !strings.Contains(strings.ToLower(j.Location), ll) {
			continue
		}
		out = append(out, *j)
	}
	return out
}

// Count returns the number of active jobs (used by the matrix counters).
func (m *memoryJobs) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, j := range m.byHash {
		if j.IsActive {
			n++
		}
	}
	return n
}

func contains(xs pq.StringArray, target string) bool {
	for _, s := range xs {
		if s == target {
			return true
		}
	}
	return false
}
