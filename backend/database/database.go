package database

import (
	"log"

	"jobscout/config"
	"jobscout/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB is the process-wide GORM handle.
var DB *gorm.DB

// Connect opens a Postgres connection using config.DatabaseURL and runs auto-migration.
// If the connection fails, Connect logs a warning and returns nil so the API
// can still serve the hardcoded mock data (Step 3).
func Connect() *gorm.DB {
	dsn := config.Load().DatabaseURL

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Printf("[db] connection failed (%v) — continuing without DB", err)
		return nil
	}

	if err := db.AutoMigrate(
		&models.Job{},
		&models.ScrapeTask{},
		&models.ScrapeResult{},
		&models.NicheProfile{},
		&models.NicheSource{},
		&models.ScraperMetric{},
		&models.ScraperLog{},
		&models.SavedJob{},
	); err != nil {
		log.Printf("[db] auto-migrate failed: %v", err)
		return nil
	}

	DB = db
	log.Println("[db] connected & migrated")
	return db
}
