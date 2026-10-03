package service

import (
	"encoding/json"
	"log"
	"time"

	"omni-tracker-api/internal/models"
	"omni-tracker-api/internal/repository"
)

// StartTaskScheduler runs an internal cron in Go to push tasks to Redis
func StartTaskScheduler() {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		// Run once immediately
		PushTasksToQueue()
		for range ticker.C {
			PushTasksToQueue()
		}
	}()
}

func PushTasksToQueue() {
	var products []models.Product
	repository.DB.Where("is_active = ?", true).Find(&products)
	
	if len(products) == 0 {
		return
	}

	var configs []models.StoreConfig
	repository.DB.Find(&configs)
	
	cfgMap := make(map[string]string)
	for _, cfg := range configs {
		cfgMap[cfg.Store] = cfg.Selector
	}

	for _, p := range products {
		task := models.TaskResponse{
			Product:  p,
			Selector: cfgMap[p.Store],
		}
		
		taskJSON, err := json.Marshal(task)
		if err == nil {
			// LPUSH puts the task into the "scraper_tasks" list
			repository.RDB.LPush(repository.Ctx, "scraper_tasks", taskJSON)
		}
	}
	log.Printf("📥 [Scheduler] Pushed %d tasks to Redis Queue.", len(products))
}
