package queue

import (
	"log"

	"jobscout/config"

	"github.com/hibiken/asynq"
)

// Start spins up the Asynq server (worker) + scheduler in background goroutines.
// If Redis is unreachable the process keeps running without background jobs.
//
// Returns a shutdown func that stops both components cleanly.
func Start() func() {
	cfg := config.Load()
	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}

	// --- Worker: consumes enqueued tasks ---
	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 4,
		Queues:      map[string]int{"default": 1},
		Logger:      asynqLogger{},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskRunScraper, HandleScraperRun)
	mux.HandleFunc(TaskDeadlineJanitor, HandleDeadlineJanitor)

	go func() {
		if err := srv.Run(mux); err != nil {
			log.Printf("[queue] asynq server stopped: %v", err)
		}
	}()

	// --- Scheduler: periodically enqueues tasks ---
	scheduler := asynq.NewScheduler(redisOpt, &asynq.SchedulerOpts{Logger: asynqLogger{}})

	// Every 6 hours: run the Python scraper.
	if _, err := scheduler.Register("@every 6h", asynq.NewTask(TaskRunScraper, nil)); err != nil {
		log.Printf("[queue] register scraper schedule failed: %v", err)
	}
	// Run the deadline janitor hourly so freshly-expired jobs leave the
	// active feed within an hour of crossing the grace window — no need
	// to wait until the next overnight pass.
	if _, err := scheduler.Register("@every 1h", asynq.NewTask(TaskDeadlineJanitor, nil)); err != nil {
		log.Printf("[queue] register janitor schedule failed: %v", err)
	}

	go func() {
		if err := scheduler.Run(); err != nil {
			log.Printf("[queue] asynq scheduler stopped: %v", err)
		}
	}()

	log.Printf("[queue] asynq server + scheduler started (redis=%s)", cfg.RedisAddr)

	return func() {
		scheduler.Shutdown()
		srv.Shutdown()
	}
}

// asynqLogger adapts Asynq's logger interface to the stdlib log package.
type asynqLogger struct{}

func (asynqLogger) Debug(args ...any) { log.Println(append([]any{"[asynq][debug]"}, args...)...) }
func (asynqLogger) Info(args ...any)  { log.Println(append([]any{"[asynq][info]"}, args...)...) }
func (asynqLogger) Warn(args ...any)  { log.Println(append([]any{"[asynq][warn]"}, args...)...) }
func (asynqLogger) Error(args ...any) { log.Println(append([]any{"[asynq][error]"}, args...)...) }
func (asynqLogger) Fatal(args ...any) { log.Println(append([]any{"[asynq][fatal]"}, args...)...) }
