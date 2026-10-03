package handler

import (
	"fmt"
	"time"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"omni-tracker-api/internal/config"
	"omni-tracker-api/internal/models"
	"omni-tracker-api/internal/repository"
	"omni-tracker-api/internal/service"
)

func GetTasks(c *fiber.Ctx) error {
	var products []models.Product
	repository.DB.Where("is_active = ?", true).Find(&products)
	
	var configs []models.StoreConfig
	repository.DB.Find(&configs)
	
	cfgMap := make(map[string]string)
	for _, cfg := range configs {
		cfgMap[cfg.Store] = cfg.Selector
	}

	var tasks []models.TaskResponse
	for _, p := range products {
		tasks = append(tasks, models.TaskResponse{
			Product:  p,
			Selector: cfgMap[p.Store],
		})
	}
	return c.JSON(tasks)
}

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
			var lastPrice float64
			_, _ = fmt.Sscanf(lastPriceStr, "%f", &lastPrice)

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
