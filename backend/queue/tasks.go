package queue

// Task type identifiers used by Asynq. Keep them as short, colon-namespaced
// strings — Asynq treats them as opaque routing keys.
const (
	TaskRunScraper      = "scraper:run"
	TaskDeadlineJanitor = "janitor:deactivate-expired"
)
