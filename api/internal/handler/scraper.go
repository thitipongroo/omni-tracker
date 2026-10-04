package handler

import (
	"fmt"
	"strconv"
	"time"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"omni-tracker-api/internal/config"
	"omni-tracker-api/internal/models"
	"omni-tracker-api/internal/repository"
	"omni-tracker-api/internal/service"
)



func UpdateStatus(c *fiber.Ctx) error {
	var payload struct {
		Status string `json:"status"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.SendStatus(400)
	}
	now := time.Now()
	repository.DB.Model(&models.Product{}).Where("id = ?", c.Params("id")).Updates(models.Product{
		Status:        payload.Status,
		LastScrapedAt: &now,
	})
	return c.SendStatus(200)
}

func PostPrice(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := new(models.PricePayload)
		if err := c.BodyParser(p); err != nil {
			return c.SendStatus(400)
		}

		cacheKey := fmt.Sprintf("price:%s:%s", p.Store, p.ProductID)
		lastPriceStr, err := repository.RDB.Get(repository.Ctx, cacheKey).Result()

		if err == redis.Nil {
			repository.RDB.Set(repository.Ctx, cacheKey, p.Price, 0)
		} else if err == nil {
			lastPrice, _ := strconv.ParseFloat(lastPriceStr, 64)

			if p.Price < lastPrice*0.2 && p.Price > 0 {
				repository.DB.Model(&models.Product{}).Where("product_id = ? AND store = ?", p.ProductID, p.Store).Update("status", "ANOMALY")
				return c.SendStatus(200) // Skip saving this weird data
			}

			if service.EvaluatePriceDrop(lastPrice, p.Price) {
				msg := fmt.Sprintf("🚨 Price Drop Alert!\n%s at %s dropped from %.2f to %.2f THB\nLink: %s",
					p.ProductID, p.Store, lastPrice, p.Price, p.URL)
				go service.SendLineAlert(cfg.LineToken, msg)
			}
			if p.Price != lastPrice {
				repository.RDB.Set(repository.Ctx, cacheKey, p.Price, 0)
			}
		}

		point := influxdb2.NewPointWithMeasurement("product_price").
			AddTag("store", p.Store).
			AddTag("product_id", p.ProductID).
			AddField("price", p.Price).
			SetTime(time.Now())
		repository.WriteAPI.WritePoint(point)

		return c.SendStatus(200)
	}
}

func GetConfigs(c *fiber.Ctx) error {
	var configs []models.StoreConfig
	repository.DB.Find(&configs)
	return c.JSON(configs)
}

func UpdateConfig(c *fiber.Ctx) error {
	cfg := new(models.StoreConfig)
	if err := c.BodyParser(cfg); err != nil {
		return c.SendStatus(400)
	}
	var existing models.StoreConfig
	if res := repository.DB.Where("store = ?", cfg.Store).First(&existing); res.Error == nil {
		existing.Selector = cfg.Selector
		repository.DB.Save(&existing)
	} else {
		repository.DB.Create(cfg)
	}
	return c.JSON(cfg)
}

func PostLog(c *fiber.Ctx) error {
	logEntry := new(models.ScrapeLog)
	if err := c.BodyParser(logEntry); err != nil {
		return c.SendStatus(400)
	}
	logEntry.CreatedAt = time.Now()
	repository.DB.Create(logEntry)
	return c.SendStatus(200)
}

func GetLogs(c *fiber.Ctx) error {
	var logs []models.ScrapeLog
	
	if userID, ok := c.Locals("userID").(uint); ok {
		var productIDs []string
		repository.DB.Model(&models.Product{}).Where("user_id = ?", userID).Pluck("product_id", &productIDs)
		
		if len(productIDs) > 0 {
			repository.DB.Where("product_id IN ?", productIDs).Order("created_at desc").Limit(100).Find(&logs)
		}
	}
	
	return c.JSON(logs)
}

