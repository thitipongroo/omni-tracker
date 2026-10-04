package repository

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var RDB *redis.Client

// InitRedis accepts either a URL (redis://:pass@host:6379/0) or a plain host:port.
func InitRedis(rawURL, password string) {
	var opts *redis.Options
	if strings.Contains(rawURL, "://") {
		parsed, err := redis.ParseURL(rawURL)
		if err != nil {
			log.Fatalf("Invalid REDIS_URL: %v", err)
		}
		opts = parsed
	} else {
		opts = &redis.Options{Addr: rawURL}
	}
	if password != "" && opts.Password == "" {
		opts.Password = password
	}
	RDB = redis.NewClient(opts)

	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := RDB.Ping(ctx).Err()
		cancel()
		if err == nil {
			return
		}
		log.Printf("Waiting for Redis... (%d/10): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	log.Fatal("Failed to connect to Redis")
}

func CloseRedis() {
	if RDB != nil {
		_ = RDB.Close()
	}
}

// Redis keys are always based on products.id (never the user-supplied product_id) to avoid cross-tenant collisions.
func PriceCacheKey(productPK uint) string   { return "price:last:" + uitoa(productPK) }
func AnomalyCountKey(productPK uint) string { return "price:anomaly:" + uitoa(productPK) }
func AnomalyPriceKey(productPK uint) string { return "price:anomaly_price:" + uitoa(productPK) }

const TaskQueueKey = "scraper_tasks"
