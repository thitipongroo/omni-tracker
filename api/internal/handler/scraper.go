package handler

import (
	"context"
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

		if p.Price <= 0 {
			repository.DB.Model(&models.Product{}).Where("id = ?", p.ID).Update("status", "FAILED")
			return c.Status(400).SendString("Invalid price: must be greater than 0")
		}

		var prod models.Product
		if err := repository.DB.First(&prod, p.ID).Error; err != nil {
			return c.Status(404).SendString("Product not found")
		}

		cacheKey := fmt.Sprintf("price:product_pk:%d", p.ID)
		ctx := context.Background()
		lastPriceStr, err := repository.RDB.Get(ctx, cacheKey).Result()

		if err == redis.Nil {
			repository.RDB.Set(ctx, cacheKey, p.Price, 0)
		} else if err == nil {
			lastPrice, _ := strconv.ParseFloat(lastPriceStr, 64)

			if p.Price < lastPrice*0.2 {
				repository.DB.Model(&models.Product{}).Where("id = ?", p.ID).Update("status", "ANOMALY")
				return c.SendStatus(422) // Stop scraper from overwriting ANOMALY with SUCCESS later
			}

			// 5% price drop threshold
			if service.EvaluatePriceDrop(lastPrice, p.Price, 5.0) {
				var u models.User
				if err := repository.DB.First(&u, prod.UserID).Error; err == nil && u.LineUserID != "" {
					msg := fmt.Sprintf("🚨 Price Drop Alert!\n%s at %s dropped from %.2f to %.2f THB\nLink: %s",
						prod.ProductID, prod.Store, lastPrice, p.Price, prod.URL)
					go func() {
						_ = service.SendLinePush(context.Background(), cfg.LineChannelToken, u.LineUserID, msg)
					}()
				}
			}
			if p.Price != lastPrice {
				repository.RDB.Set(ctx, cacheKey, p.Price, 0)
			}
		}

		point := influxdb2.NewPointWithMeasurement("product_price").
			AddTag("store", prod.Store).
			AddTag("product_pk", strconv.Itoa(int(p.ID))).
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
	
	if !validLogLevels[logEntry.Level] {
		logEntry.Level = "INFO"
	}
	
	repository.DB.Create(logEntry)
	return c.SendStatus(200)
}

func GetLogs(c *fiber.Ctx) error {
	var logs []models.ScrapeLog
	
	userID, ok := currentUserID(c)
	if !ok {
		return jsonError(c, fiber.StatusUnauthorized, "Unauthorized")
	}
	
	var productIDs []uint
	repository.DB.Model(&models.Product{}).Where("user_id = ?", userID).Pluck("id", &productIDs)
	
	if len(productIDs) > 0 {
		repository.DB.Where("product_pk IN ?", productIDs).Order("created_at desc").Limit(100).Find(&logs)
	}
	
	return c.JSON(logs)
}
