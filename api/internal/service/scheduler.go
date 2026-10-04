package service

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"omni-tracker-api/internal/models"
	"omni-tracker-api/internal/repository"
	"gorm.io/gorm"
)

// StartTaskScheduler runs an internal cron in Go to push tasks to Redis
func StartTaskScheduler(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		defer ticker.Stop()
		// Run once immediately
		tryPushTasks()
		for {
			select {
			case <-ticker.C:
				tryPushTasks()
			case <-ctx.Done():
				log.Println("Stopping Task Scheduler...")
				return
			}
		}
	}()
}

func tryPushTasks() {
	lockKey := "scheduler:lock"
	// Try to acquire lock for 9 minutes (just below the 10 min interval)
	acquired, err := repository.RDB.SetNX(repository.Ctx, lockKey, "locked", 9*time.Minute).Result()
	if err != nil || !acquired {
		log.Println("📥 [Scheduler] Another instance is running the scheduler. Skipping.")
		return
	}
	PushTasksToQueue()
}

func PushTasksToQueue() {
	// Prevent Queue Pile-up: Check if queue is already too large
	qLen, err := repository.RDB.LLen(repository.Ctx, "scraper_tasks").Result()
	if err == nil && qLen > 2000 {
		log.Printf("⚠️ [Scheduler] Queue is too large (%d). Skipping this cycle to prevent pile-up.", qLen)
		return
	}

	var configs []models.StoreConfig
	repository.DB.Find(&configs)
	
	cfgMap := make(map[string]string)
	for _, cfg := range configs {
		cfgMap[cfg.Store] = cfg.Selector
	}

	var total int
	// Prevent duplicate scheduling: Only select products that haven't been scraped in the last 9 minutes
	timeThreshold := time.Now().Add(-9 * time.Minute)
	
	repository.DB.Where("is_active = ? AND status != 'QUEUED' AND (last_scraped_at IS NULL OR last_scraped_at < ?)", true, timeThreshold).
		FindInBatches(&[]models.Product{}, 500, func(tx *gorm.DB, batch int) error {
		var products []models.Product
		tx.Scan(&products)
		
		for _, p := range products {
			task := models.TaskResponse{
				Product:  p,
				Selector: cfgMap[p.Store],
			}
			
			taskJSON, err := json.Marshal(task)
			if err == nil {
				repository.RDB.LPush(repository.Ctx, "scraper_tasks", taskJSON)
				repository.DB.Model(&models.Product{}).Where("id = ?", p.ID).Update("status", "QUEUED")
			}
		}
		total += len(products)
		return nil
	})

	log.Printf("📥 [Scheduler] Pushed %d tasks to Redis Queue.", total)
}
