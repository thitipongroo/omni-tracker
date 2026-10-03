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
		PushTasksToQueue()
		for {
			select {
			case <-ticker.C:
				PushTasksToQueue()
			case <-ctx.Done():
				log.Println("Stopping Task Scheduler...")
				return
			}
		}
	}()
}

func PushTasksToQueue() {
	var configs []models.StoreConfig
	repository.DB.Find(&configs)
	
	cfgMap := make(map[string]string)
	for _, cfg := range configs {
		cfgMap[cfg.Store] = cfg.Selector
	}

	var total int
	repository.DB.Where("is_active = ?", true).FindInBatches(&[]models.Product{}, 500, func(tx *gorm.DB, batch int) error {
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
			}
		}
		total += len(products)
		return nil
	})

	log.Printf("📥 [Scheduler] Pushed %d tasks to Redis Queue.", total)
}
