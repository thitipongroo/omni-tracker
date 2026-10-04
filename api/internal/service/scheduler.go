package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"omni-tracker-api/internal/config"
	"omni-tracker-api/internal/models"
	"omni-tracker-api/internal/repository"
)

const (
	maxQueueLength = 2000
	batchSize      = 500
)

var errQueueFull = errors.New("queue capacity reached")

// StartTaskScheduler runs the internal cron that pushes scrape tasks to Redis and prunes old logs.
// The returned channel is closed once the scheduler goroutine has fully stopped (after ctx is cancelled).
func StartTaskScheduler(ctx context.Context, cfg *config.Config) <-chan struct{} {
	done := make(chan struct{})
	interval := time.Duration(cfg.ScrapeIntervalMin) * time.Minute

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		cleanup := time.NewTicker(time.Hour)
		defer cleanup.Stop()

		tryPushTasks(ctx, cfg)
		tryCleanupLogs(ctx, cfg)
		for {
			select {
			case <-ticker.C:
				tryPushTasks(ctx, cfg)
			case <-cleanup.C:
				tryCleanupLogs(ctx, cfg)
			case <-ctx.Done():
				log.Println("Stopping Task Scheduler...")
				return
			}
		}
	}()
	return done
}

func tryPushTasks(ctx context.Context, cfg *config.Config) {
	interval := time.Duration(cfg.ScrapeIntervalMin) * time.Minute
	lockTTL := interval - interval/10 // just below one interval
	acquired, err := repository.RDB.SetNX(ctx, "scheduler:lock", "locked", lockTTL).Result()
	if err != nil {
		log.Printf("⚠️ [Scheduler] Could not acquire lock: %v", err)
		return
	}
	if !acquired {
		log.Println("📥 [Scheduler] Another instance is running the scheduler. Skipping.")
		return
	}
	if err := PushTasksToQueue(ctx, cfg); err != nil {
		log.Printf("❌ [Scheduler] %v", err)
	}
}

// PushTasksToQueue enqueues every active product that is due for a scrape.
//
// Products stuck in QUEUED (worker crashed, pod scaled down, Redis push lost...) are
// re-queued once their queued_at is older than STALE_QUEUE_MINUTES, so nothing is stuck forever.
func PushTasksToQueue(ctx context.Context, cfg *config.Config) error {
	qLen, err := repository.RDB.LLen(ctx, repository.TaskQueueKey).Result()
	if err != nil {
		return fmt.Errorf("read queue length: %w", err)
	}
	remaining := maxQueueLength - int(qLen)
	if remaining <= 0 {
		log.Printf("⚠️ [Scheduler] Queue is too large (%d). Skipping this cycle; add workers or raise SCRAPE_INTERVAL_MINUTES.", qLen)
		return nil
	}

	var configs []models.StoreConfig
	if err := repository.DB.WithContext(ctx).Find(&configs).Error; err != nil {
		return fmt.Errorf("load store configs: %w", err)
	}
	selectors := make(map[string]string, len(configs))
	for _, c := range configs {
		selectors[c.Store] = c.Selector
	}

	interval := time.Duration(cfg.ScrapeIntervalMin) * time.Minute
	now := time.Now()
	dueBefore := now.Add(-(interval - interval/10))
	staleBefore := now.Add(-time.Duration(cfg.StaleQueueMin) * time.Minute)

	total := 0
	var batch []models.Product
	res := repository.DB.WithContext(ctx).
		Select("id", "product_id", "store", "url").
		Where("is_active = ?", true).
		Where("(last_scraped_at IS NULL OR last_scraped_at < ?)", dueBefore).
		Where("(status <> ? OR queued_at IS NULL OR queued_at < ?)", models.StatusQueued, staleBefore).
		FindInBatches(&batch, batchSize, func(tx *gorm.DB, _ int) error {
			pipe := repository.RDB.Pipeline()
			ids := make([]uint, 0, len(batch))
			for _, p := range batch {
				if total+len(ids) >= remaining {
					break
				}
				payload, err := json.Marshal(models.ScrapeTask{
					ID: p.ID, ProductID: p.ProductID, Store: p.Store, URL: p.URL, Selector: selectors[p.Store],
				})
				if err != nil {
					continue
				}
				pipe.LPush(ctx, repository.TaskQueueKey, payload)
				ids = append(ids, p.ID)
			}
			if len(ids) == 0 {
				return errQueueFull
			}
			// Only mark as QUEUED after Redis confirmed the push.
			if _, err := pipe.Exec(ctx); err != nil {
				return fmt.Errorf("push tasks to redis: %w", err)
			}
			if err := repository.DB.WithContext(ctx).Model(&models.Product{}).
				Where("id IN ?", ids).
				Updates(map[string]any{"status": models.StatusQueued, "queued_at": time.Now()}).Error; err != nil {
				return fmt.Errorf("mark products queued: %w", err)
			}
			total += len(ids)
			if total >= remaining {
				return errQueueFull
			}
			return nil
		})

	if res.Error != nil && !errors.Is(res.Error, errQueueFull) {
		return res.Error
	}
	log.Printf("📥 [Scheduler] Pushed %d tasks to Redis Queue.", total)
	return nil
}

func tryCleanupLogs(ctx context.Context, cfg *config.Config) {
	acquired, err := repository.RDB.SetNX(ctx, "scheduler:log_cleanup_lock", "locked", 23*time.Hour).Result()
	if err != nil || !acquired {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -cfg.LogRetentionDays)
	res := repository.DB.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&models.ScrapeLog{})
	if res.Error != nil {
		log.Printf("❌ [Scheduler] Log cleanup failed: %v", res.Error)
		return
	}
	log.Printf("🧹 [Scheduler] Deleted %d scrape logs older than %d days.", res.RowsAffected, cfg.LogRetentionDays)
}
