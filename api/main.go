package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Product struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProductID string    `gorm:"not null" json:"product_id"`
	Store     string    `gorm:"not null" json:"store"`
	URL       string    `gorm:"not null" json:"url"`
	IsActive  bool      `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

type PricePayload struct {
	ProductID string  `json:"product_id"`
	Store     string  `json:"store"`
	Price     float64 `json:"price"`
	URL       string  `json:"url"`
}

var (
	db    *gorm.DB
	rdb   *redis.Client
	ctxBg = context.Background()
)

func main() {
	app := fiber.New()
	app.Use(cors.New()) // Enable CORS for Dashboard

	influxURL := getEnv("INFLUXDB_URL", "http://influxdb:8086")
	influxToken := getEnv("INFLUXDB_TOKEN", "super-secret-token")
	influxOrg := getEnv("INFLUXDB_ORG", "my-org")
	influxBucket := getEnv("INFLUXDB_BUCKET", "market-data")
	apiKey := getEnv("API_KEY", "my-internal-secret-key")
	lineToken := getEnv("LINE_NOTIFY_TOKEN", "")
	dsn := getEnv("DATABASE_URL", "host=postgres user=admin password=admin dbname=omnitracker port=5432 sslmode=disable")
	redisURL := getEnv("REDIS_URL", "redis:6379")

	// Init PostgreSQL
	var err error
	for i := 0; i < 5; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		log.Println("Waiting for PostgreSQL to start...")
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatal("Failed to connect to DB:", err)
	}
	db.AutoMigrate(&Product{}) // Migrate Database Schema

	// Init Redis Cache
	rdb = redis.NewClient(&redis.Options{
		Addr: redisURL,
	})

	// Init InfluxDB Client
	client := influxdb2.NewClient(influxURL, influxToken)
	writeAPI := client.WriteAPI(influxOrg, influxBucket)

	// Serve Static Dashboard Files
	app.Static("/", "./dashboard")

	apiGroup := app.Group("/api")

	// === Dashboard Endpoints ===
	apiGroup.Get("/products", func(c *fiber.Ctx) error {
		var products []Product
		db.Find(&products)
		return c.JSON(products)
	})

	apiGroup.Post("/products", func(c *fiber.Ctx) error {
		p := new(Product)
		if err := c.BodyParser(p); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
		}
		db.Create(p)
		return c.JSON(p)
	})

	apiGroup.Delete("/products/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		db.Delete(&Product{}, id)
		return c.SendStatus(200)
	})

	// Auth Middleware for Scraper Endpoints
	authMiddleware := func(c *fiber.Ctx) error {
		token := c.Get("Authorization")
		if token != "Bearer "+apiKey {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		return c.Next()
	}

	// === Scraper Endpoints ===
	apiGroup.Get("/tasks", authMiddleware, func(c *fiber.Ctx) error {
		var products []Product
		db.Where("is_active = ?", true).Find(&products)
		return c.JSON(products)
	})

	apiGroup.Post("/prices", authMiddleware, func(c *fiber.Ctx) error {
		p := new(PricePayload)
		if err := c.BodyParser(p); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
		}

		// 1. REDIS CACHE: Fast query for Alerting Engine
		cacheKey := fmt.Sprintf("price:%s:%s", p.Store, p.ProductID)
		lastPriceStr, err := rdb.Get(ctxBg, cacheKey).Result()

		if err == redis.Nil {
			// First time tracking this product
			rdb.Set(ctxBg, cacheKey, p.Price, 0)
		} else if err == nil {
			// Compare with cached price
			var lastPrice float64
			fmt.Sscanf(lastPriceStr, "%f", &lastPrice)

			if p.Price < lastPrice {
				msg := fmt.Sprintf("🚨 Price Drop Alert!\n%s at %s dropped from %.2f to %.2f THB\nLink: %s",
					p.ProductID, p.Store, lastPrice, p.Price, p.URL)
				go sendLineAlert(lineToken, msg)
			}

			// Update cache if price changed
			if p.Price != lastPrice {
				rdb.Set(ctxBg, cacheKey, p.Price, 0)
			}
		}

		// 2. INFLUXDB: Async write for historical trends
		point := influxdb2.NewPointWithMeasurement("product_price").
			AddTag("store", p.Store).
			AddTag("product_id", p.ProductID).
			AddField("price", p.Price).
			SetTime(time.Now())
		writeAPI.WritePoint(point)

		return c.SendStatus(fiber.StatusOK)
	})

	port := getEnv("API_PORT", "3000")
	log.Fatal(app.Listen(":" + port))
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func sendLineAlert(token, message string) {
	if token == "" || token == "your_line_notify_token_here" {
		return
	}
	apiURL := "https://notify-api.line.me/api/notify"
	data := url.Values{}
	data.Set("message", message)
	
	req, _ := http.NewRequest("POST", apiURL, strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	
	client := &http.Client{Timeout: 10 * time.Second}
	client.Do(req)
}
