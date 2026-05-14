package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// Job is the canonical job record served by the API and persisted in Postgres.
// The JSON tags are camelCase so the shape matches the TypeScript `Job`
// interface consumed by the Lovable frontend (`frontend/src/lib/jobs-data.ts`).
type Job struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Title         string         `gorm:"not null" json:"title"`
	Company       string         `gorm:"not null" json:"company"`
	Location      string         `gorm:"not null" json:"location"`
	URL           string         `json:"url,omitempty"`
	Salary        string         `json:"salary,omitempty"`
	PostedAt      time.Time      `json:"postedAt"`
	Deadline      *time.Time     `json:"deadline,omitempty"`
	MatchScore    int            `json:"matchScore"`
	Sources       pq.StringArray `gorm:"type:text[]" json:"sources"`
	Tags          pq.StringArray `gorm:"type:text[]" json:"tags"`
	Type          string         `json:"type"`  // Remote | Hybrid | Onsite
	Level         string         `json:"level"` // Junior | Mid | Senior | Lead
	SourceHash    string         `gorm:"uniqueIndex" json:"-"`
	MergedURLs    pq.StringArray `gorm:"type:text[]" json:"mergedUrls,omitempty"`
	MergeCount    int            `gorm:"default:0" json:"mergeCount"`
	IsActive      bool           `gorm:"default:true;index" json:"isActive"`
	LastScrapedAt *time.Time     `json:"lastScrapedAt,omitempty"`
	// NicheID links this job to the NicheProfile that produced it
	// (nil for legacy / un-niched jobs). Used by the Job Matrix niche filter.
	NicheID   *uuid.UUID `gorm:"type:uuid;index" json:"nicheId,omitempty"`
	CreatedAt time.Time  `json:"-"`
	UpdatedAt time.Time  `json:"-"`
}

// BeforeCreate ensures every record has a UUID primary key.
func (j *Job) BeforeCreate(tx *gorm.DB) error {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	return nil
}

// --- User-defined scraping targets -------------------------------------------------

// ScrapeTaskType is the enum of targeting strategies a user can configure.
type ScrapeTaskType string

const (
	TaskTypeKeyword   ScrapeTaskType = "KEYWORD"
	TaskTypeDirectURL ScrapeTaskType = "DIRECT_URL"
)

// ScrapeTaskStatus tracks the health of a target's most recent run.
type ScrapeTaskStatus string

const (
	StatusPending  ScrapeTaskStatus = "pending"
	StatusRunning  ScrapeTaskStatus = "running"
	StatusHealthy  ScrapeTaskStatus = "healthy"
	StatusFailed   ScrapeTaskStatus = "failed"
)

// ScrapeTask is a single user-configured scraping target. The scheduler
// picks them up by Frequency and the manual API lets users trigger
// individual runs from the Targeting Dashboard.
type ScrapeTask struct {
	ID          uuid.UUID        `gorm:"type:uuid;primaryKey" json:"id"`
	Type        ScrapeTaskType   `gorm:"type:varchar(20);not null;index" json:"type"`
	Value       string           `gorm:"not null" json:"value"`
	Frequency   string           `gorm:"default:'6h'" json:"frequency"` // 'hourly' | '6h' | 'daily'
	Status      ScrapeTaskStatus `gorm:"type:varchar(20);default:'pending'" json:"status"`
	LastRunAt   *time.Time       `json:"lastRunAt,omitempty"`
	LastError   string           `json:"lastError,omitempty"`
	ResultCount int              `gorm:"default:0" json:"resultCount"`
	IsActive    bool             `gorm:"default:true;index" json:"isActive"`
	// NicheID associates this target with a NicheProfile so the Targeted
	// Dispatcher only fires it (and its ContextKeyword validator) for the
	// matching niche.
	NicheID   *uuid.UUID `gorm:"type:uuid;index" json:"nicheId,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"-"`
}

func (t *ScrapeTask) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

// ScrapeResult links a Job back to the ScrapeTask that produced it.
// One Job may be linked to multiple tasks (cross-source merge), so this
// is intentionally a join row rather than an FK on Job.
type ScrapeResult struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TaskID  uuid.UUID `gorm:"type:uuid;index;not null" json:"taskId"`
	JobID   uuid.UUID `gorm:"type:uuid;index;not null" json:"jobId"`
	FoundAt time.Time `json:"foundAt"`
}

func (r *ScrapeResult) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	if r.FoundAt.IsZero() {
		r.FoundAt = time.Now().UTC()
	}
	return nil
}

// --- Niche scoping (Phase 4) -------------------------------------------------

// NicheProfile groups targets and validation keywords under a domain
// (e.g. "Architecture", "Civil Engineering", "Computer Science"). Every
// scraped Job is validated against its niche's ContextKeywords before
// being persisted, which prevents cross-domain false positives like
// "Software Architect" leaking into an "Architecture" niche.
type NicheProfile struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Name            string         `gorm:"uniqueIndex;not null" json:"name"`
	Description     string         `json:"description,omitempty"`
	SeedKeywords    pq.StringArray `gorm:"type:text[]" json:"seedKeywords"`
	ContextKeywords pq.StringArray `gorm:"type:text[]" json:"contextKeywords"`
	// MinContextMatches is the validation threshold: a scraped job must
	// contain at least this many ContextKeywords to be persisted under
	// this niche. Defaults to 2 per the spec.
	MinContextMatches int       `gorm:"default:2" json:"minContextMatches"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"-"`
}

func (n *NicheProfile) BeforeCreate(tx *gorm.DB) error {
	if n.ID == uuid.Nil {
		n.ID = uuid.New()
	}
	return nil
}

// NicheSource is a curated URL associated with a NicheProfile. Conceptually
// it's a ScrapeTask of type DIRECT_URL pre-bound to a niche; we keep it as
// a separate row so users can manage them from the Niche Manager page
// without polluting the Targeting Dashboard.
type NicheSource struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	NicheID   uuid.UUID `gorm:"type:uuid;index;not null" json:"nicheId"`
	URL       string    `gorm:"not null" json:"url"`
	Label     string    `json:"label,omitempty"` // optional pretty name; falls back to hostname
	IsActive  bool      `gorm:"default:true" json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *NicheSource) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

// --- Scraper telemetry ------------------------------------------------------

// ScraperMetricEvent classifies a single scraper outcome so the Health
// Matrix can compute an error rate (`failed / total`) and break down
// niche-filter rejections separately from true failures.
type ScraperMetricEvent string

const (
	MetricSuccess  ScraperMetricEvent = "success"
	MetricFailure  ScraperMetricEvent = "failure"
	MetricRejected ScraperMetricEvent = "rejected" // passed niche filter check but didn't meet threshold
)

// ScraperMetric is one row per scraper decision: a successful run, a
// Python/network failure, or a job rejected by the Niche context filter.
// Rolled up by the /api/scraper/metrics endpoint to drive the ERR RATE
// column on the Scraper Health Matrix.
type ScraperMetric struct {
	ID        uuid.UUID          `gorm:"type:uuid;primaryKey" json:"id"`
	TaskID    *uuid.UUID         `gorm:"type:uuid;index" json:"taskId,omitempty"`
	NicheID   *uuid.UUID         `gorm:"type:uuid;index" json:"nicheId,omitempty"`
	Event     ScraperMetricEvent `gorm:"type:varchar(20);index;not null" json:"event"`
	Message   string             `json:"message,omitempty"`
	CreatedAt time.Time          `gorm:"index" json:"createdAt"`
}

func (m *ScraperMetric) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	return nil
}

// ScraperLog captures errors and details from scraper runs for debugging.
// One row per failed Python invocation: the task id (or "test"/"manual"),
// the short error reason (e.g. "exit status 1", "context deadline exceeded"),
// and the captured stderr (truncated to ~16 KB) for the UI to render.
type ScraperLog struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TaskID    string     `gorm:"type:varchar(64);index" json:"taskId,omitempty"`
	NicheID   *uuid.UUID `gorm:"type:uuid;index" json:"nicheId,omitempty"`
	URL       string     `json:"url,omitempty"`
	Stage     string     `gorm:"type:varchar(32);index" json:"stage,omitempty"` // spawn|extract|validate|commit
	Error     string     `gorm:"type:text" json:"error,omitempty"`
	Details   string     `gorm:"type:text" json:"details,omitempty"` // captured stderr
	ExitCode  int        `json:"exitCode"`
	CreatedAt time.Time  `gorm:"index" json:"createdAt"`
}

func (l *ScraperLog) BeforeCreate(tx *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now().UTC()
	}
	return nil
}

// --- Saved jobs (persisted bookmarks) ---------------------------------------

// SavedJob persists a user's bookmarked Job so it survives cache clears.
// The app has no real auth, so UserKey is an opaque string the client
// sends in the X-Scout-User header (defaults to a localStorage UUID, but
// the profile widget upgrades it to the user's email so the same saved
// list follows them across devices/browsers if they sign in with the
// same email).
type SavedJob struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	UserKey   string    `gorm:"type:varchar(200);index:idx_saved_user_job,unique;not null" json:"userKey"`
	JobID     uuid.UUID `gorm:"type:uuid;index:idx_saved_user_job,unique;not null" json:"jobId"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *SavedJob) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	return nil
}
