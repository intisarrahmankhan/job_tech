package store

import (
	"sort"
	"sync"
	"time"

	"jobscout/models"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// MemoryNiches mirrors NicheProfile + NicheSource in memory for the no-DB
// dev path. The Niche Manager UI hits the same handlers regardless.
var MemoryNiches = newMemoryNiches()

type memoryNiches struct {
	mu       sync.RWMutex
	profiles map[uuid.UUID]*models.NicheProfile
	sources  map[uuid.UUID]*models.NicheSource // keyed by source id
}

func newMemoryNiches() *memoryNiches {
	return &memoryNiches{
		profiles: make(map[uuid.UUID]*models.NicheProfile),
		sources:  make(map[uuid.UUID]*models.NicheSource),
	}
}

// --- profiles ---------------------------------------------------------------

func (m *memoryNiches) ListProfiles() []models.NicheProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]models.NicheProfile, 0, len(m.profiles))
	for _, v := range m.profiles {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (m *memoryNiches) GetProfile(id uuid.UUID) *models.NicheProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.profiles[id]; ok {
		copy := *p
		return &copy
	}
	return nil
}

func (m *memoryNiches) CreateProfile(p models.NicheProfile) models.NicheProfile {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	if p.MinContextMatches == 0 {
		p.MinContextMatches = 2
	}
	if p.SeedKeywords == nil {
		p.SeedKeywords = pq.StringArray{}
	}
	if p.ContextKeywords == nil {
		p.ContextKeywords = pq.StringArray{}
	}
	stored := p
	m.profiles[p.ID] = &stored
	return stored
}

func (m *memoryNiches) UpdateProfile(id uuid.UUID, mut func(*models.NicheProfile)) *models.NicheProfile {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.profiles[id]
	if !ok {
		return nil
	}
	mut(p)
	p.UpdatedAt = time.Now().UTC()
	copy := *p
	return &copy
}

func (m *memoryNiches) DeleteProfile(id uuid.UUID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.profiles[id]; !ok {
		return false
	}
	delete(m.profiles, id)
	// Cascade: drop every source bound to this niche.
	for sid, s := range m.sources {
		if s.NicheID == id {
			delete(m.sources, sid)
		}
	}
	return true
}

// --- sources ----------------------------------------------------------------

func (m *memoryNiches) ListSources(nicheID uuid.UUID) []models.NicheSource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]models.NicheSource, 0)
	for _, s := range m.sources {
		if s.NicheID == nicheID {
			out = append(out, *s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (m *memoryNiches) CreateSource(s models.NicheSource) models.NicheSource {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	s.IsActive = true
	stored := s
	m.sources[s.ID] = &stored
	return stored
}

func (m *memoryNiches) DeleteSource(id uuid.UUID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sources[id]; !ok {
		return false
	}
	delete(m.sources, id)
	return true
}

// GetSource returns a snapshot copy of a single source, or nil if missing.
// Used by DeleteNicheSource so the cascade can read the URL + niche ID
// before issuing the actual delete.
func (m *memoryNiches) GetSource(id uuid.UUID) *models.NicheSource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.sources[id]; ok {
		copy := *s
		return &copy
	}
	return nil
}
