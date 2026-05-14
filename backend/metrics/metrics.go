// Package metrics records scraper telemetry (success / failure / niche
// rejection) so the Health Matrix can surface an ERR RATE column and the
// System Pulse can show niche-scoped rollups like
// "Niche Architecture validated 5 new roles".
//
// All recording is best-effort: if the DB is unavailable we keep a small
// in-memory ring so dev machines without Postgres still see live numbers.
package metrics

import (
	"fmt"
	"log"
	"sync"
	"time"

	"jobscout/database"
	"jobscout/models"

	"github.com/google/uuid"
)

// Event types mirror models.ScraperMetricEvent so callers don't have to
// import models just to record a data point.
type Event = models.ScraperMetricEvent

const (
	Success  = models.MetricSuccess
	Failure  = models.MetricFailure
	Rejected = models.MetricRejected
)

// Record writes one metric row. TaskID / NicheID are optional. Safe to
// call with nil IDs. Returns silently on any error so the hot scraper
// path is never blocked by telemetry.
func Record(taskID *uuid.UUID, nicheID *uuid.UUID, event Event, message string) {
	m := models.ScraperMetric{
		TaskID:    taskID,
		NicheID:   nicheID,
		Event:     event,
		Message:   message,
		CreatedAt: time.Now().UTC(),
	}
	if database.DB != nil {
		if err := database.DB.Create(&m).Error; err != nil {
			log.Printf("[metrics] record failed: %v", err)
		}
		return
	}
	memory.push(m)
}

// TaskRollup is the per-target aggregate the /api/scraper/metrics
// endpoint returns and the Scraper Health UI consumes.
type TaskRollup struct {
	TaskID   string  `json:"taskId"`
	Total    int     `json:"total"`
	Success  int     `json:"success"`
	Failure  int     `json:"failure"`
	Rejected int     `json:"rejected"`
	ErrRate  float64 `json:"errRate"` // failure / max(1, total) as percentage 0..100
}

// RollupByTask returns one TaskRollup per task that has any metric in the
// last `since` window. Passing 0 for `since` returns the all-time rollup.
func RollupByTask(since time.Duration) map[string]TaskRollup {
	out := map[string]TaskRollup{}
	rows := snapshot(since)
	for _, r := range rows {
		if r.TaskID == nil {
			continue
		}
		id := r.TaskID.String()
		agg := out[id]
		agg.TaskID = id
		agg.Total++
		switch r.Event {
		case Success:
			agg.Success++
		case Failure:
			agg.Failure++
		case Rejected:
			agg.Rejected++
		}
		out[id] = agg
	}
	for id, agg := range out {
		if agg.Total > 0 {
			agg.ErrRate = float64(agg.Failure) / float64(agg.Total) * 100.0
			out[id] = agg
		}
	}
	return out
}

// NicheBatch counts success/rejection per niche over `since`. Used by
// the Niche Manager "Run" button to display "validated X · rejected Y".
type NicheBatch struct {
	NicheID  string `json:"nicheId"`
	Success  int    `json:"success"`
	Rejected int    `json:"rejected"`
	Failure  int    `json:"failure"`
}

func RollupByNiche(since time.Duration) map[string]NicheBatch {
	out := map[string]NicheBatch{}
	rows := snapshot(since)
	for _, r := range rows {
		if r.NicheID == nil {
			continue
		}
		id := r.NicheID.String()
		agg := out[id]
		agg.NicheID = id
		switch r.Event {
		case Success:
			agg.Success++
		case Rejected:
			agg.Rejected++
		case Failure:
			agg.Failure++
		}
		out[id] = agg
	}
	return out
}

// ---------------------------------------------------------------------------
// in-memory fallback

const memoryCap = 5000

type memoryStore struct {
	mu   sync.RWMutex
	rows []models.ScraperMetric
}

func (s *memoryStore) push(m models.ScraperMetric) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, m)
	if len(s.rows) > memoryCap {
		s.rows = s.rows[len(s.rows)-memoryCap:]
	}
}

var memory = &memoryStore{}

func snapshot(since time.Duration) []models.ScraperMetric {
	if database.DB != nil {
		var rows []models.ScraperMetric
		tx := database.DB.Model(&models.ScraperMetric{})
		if since > 0 {
			tx = tx.Where("created_at >= ?", time.Now().UTC().Add(-since))
		}
		if err := tx.Find(&rows).Error; err != nil {
			log.Printf("[metrics] snapshot failed: %v", err)
			return nil
		}
		return rows
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	if since == 0 {
		out := make([]models.ScraperMetric, len(memory.rows))
		copy(out, memory.rows)
		return out
	}
	cutoff := time.Now().UTC().Add(-since)
	out := make([]models.ScraperMetric, 0, len(memory.rows))
	for _, r := range memory.rows {
		if r.CreatedAt.After(cutoff) {
			out = append(out, r)
		}
	}
	return out
}

// FormatRollup makes a short human-readable summary for pulse lines.
func FormatRollup(label string, success, rejected, failure int) string {
	switch {
	case failure > 0:
		return fmt.Sprintf("%s · %d saved · %d rejected · %d failed", label, success, rejected, failure)
	case rejected > 0:
		return fmt.Sprintf("%s · %d saved · %d rejected by context filter", label, success, rejected)
	default:
		return fmt.Sprintf("%s · %d saved", label, success)
	}
}
