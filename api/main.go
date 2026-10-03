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

type StoreConfig struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Store    string `gorm:"uniqueIndex;not null" json:"store"`
	Selector string `gorm:"not null" json:"selector"`
}

type Product struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	ProductID     string     `gorm:"not null" json:"product_id"`
	Store         string     `gorm:"not null" json:"store"`
	URL           string     `gorm:"not null" json:"url"`
	IsActive      bool       `gorm:"default:true" json:"is_active"`
	Status        string     `gorm:"default:'PENDING'" json:"status"`
	LastScrapedAt *time.Time `json:"last_scraped_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

type TaskResponse struct {
	Product
	Selector string `json:"selector"`
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

// EvaluatePriceDrop is extracted for Unit Testing
func EvaluatePriceDrop(lastPrice, newPrice float64) bool {
	if lastPrice > 0 && newPrice < lastPrice {
		return true
	}
	return false
}

func main() {
	app := fiber.New()
	app.Use(cors.New())

	influxURL := getEnv("INFLUXDB_URL", "http://influxdb:8086")
	influxToken := getEnv("INFLUXDB_TOKEN", "super-secret-token")
	influxOrg := getEnv("INFLUXDB_ORG", "my-org")
	influxBucket := getEnv("INFLUXDB_BUCKET", "market-data")
	apiKey := getEnv("API_KEY", "my-internal-secret-key")
	adminPass := getEnv("ADMIN_PASSWORD", "admin123")
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
	db.AutoMigrate(&Product{}, &StoreConfig{})

	// Seed default configs if empty
	var count int64
	db.Model(&StoreConfig{}).Count(&count)
	if count == 0 {
		db.Create(&StoreConfig{Store: "shopee", Selector: ".product-price"})
		db.Create(&StoreConfig{Store: "lazada", Selector: ".pdp-price"})
	}

	// Init Redis Cache
	rdb = redis.NewClient(&redis.Options{Addr: redisURL})

	// Init InfluxDB Client
	client := influxdb2.NewClient(influxURL, influxToken)
	writeAPI := client.WriteAPI(influxOrg, influxBucket)
	queryAPI := client.QueryAPI(influxOrg)

	app.Static("/", "./dashboard")

	apiGroup := app.Group("/api")

	// 3. Login Endpoint (Auth)
	apiGroup.Post("/login", func(c *fiber.Ctx) error {
		var payload struct {
			Password string `json:"password"`
		}
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(400).SendString("Invalid body")
		}
		if payload.Password == adminPass {
			return c.JSON(fiber.Map{"token": apiKey})
		}
		return c.Status(401).SendString("Unauthorized")
	})

	authMiddleware := func(c *fiber.Ctx) error {
		token := c.Get("Authorization")
		if token != "Bearer "+apiKey {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		return c.Next()
	}

	protected := apiGroup.Group("", authMiddleware)

	// Product CRUD
	protected.Get("/products", func(c *fiber.Ctx) error {
		var products []Product
		db.Order("id desc").Find(&products)
		return c.JSON(products)
	})

	protected.Post("/products", func(c *fiber.Ctx) error {
		p := new(Product)
		if err := c.BodyParser(p); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
		}
		p.Status = "PENDING"
		db.Create(p)
		return c.JSON(p)
	})

	protected.Delete("/products/:id", func(c *fiber.Ctx) error {
		db.Delete(&Product{}, c.Params("id"))
		return c.SendStatus(200)
	})

	// 5 & 6. Status Update Endpoint
	protected.Patch("/products/:id/status", func(c *fiber.Ctx) error {
		var payload struct {
			Status string `json:"status"`
		}
		if err := c.BodyParser(&payload); err != nil {
			return c.SendStatus(400)
		}
		now := time.Now()
		db.Model(&Product{}).Where("id = ?", c.Params("id")).Updates(Product{
			Status:        payload.Status,
			LastScrapedAt: &now,
		})
		return c.SendStatus(200)
	})

	// 4. Dynamic Store Configs
	protected.Get("/configs", func(c *fiber.Ctx) error {
		var configs []StoreConfig
		db.Find(&configs)
		return c.JSON(configs)
	})
	
	protected.Post("/configs", func(c *fiber.Ctx) error {
		cfg := new(StoreConfig)
		c.BodyParser(cfg)
		var existing StoreConfig
		if res := db.Where("store = ?", cfg.Store).First(&existing); res.Error == nil {
			existing.Selector = cfg.Selector
			db.Save(&existing)
		} else {
			db.Create(cfg)
		}
		return c.JSON(cfg)
	})

	// Tasks Endpoint for Scraper
	protected.Get("/tasks", func(c *fiber.Ctx) error {
		var products []Product
		db.Where("is_active = ?", true).Find(&products)
		
		var configs []StoreConfig
		db.Find(&configs)
		
		cfgMap := make(map[string]string)
		for _, cfg := range configs {
			cfgMap[cfg.Store] = cfg.Selector
		}

		var tasks []TaskResponse
		for _, p := range products {
			tasks = append(tasks, TaskResponse{
				Product:  p,
				Selector: cfgMap[p.Store],
			})
		}
		return c.JSON(tasks)
	})

	// 1 & 8. InfluxDB History Query
	protected.Get("/history/:product_id", func(c *fiber.Ctx) error {
		pid := c.Params("product_id")
		query := fmt.Sprintf(`from(bucket:"%s") 
			|> range(start: -14d) 
			|> filter(fn: (r) => r._measurement == "product_price" and r.product_id == "%s")
			|> aggregateWindow(every: 1d, fn: last, createEmpty: false)
			|> yield(name: "last")`, influxBucket, pid)
		
		result, err := queryAPI.Query(ctxBg, query)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		
		type Point struct {
			Time  time.Time `json:"time"`
			Price float64   `json:"price"`
		}
		var points []Point
		for result.Next() {
			if val, ok := result.Record().Value().(float64); ok {
				points = append(points, Point{
					Time:  result.Record().Time(),
					Price: val,
				})
			}
		}
		if result.Err() != nil {
			return c.Status(500).SendString(result.Err().Error())
		}
		return c.JSON(points)
	})

	// Price Ingestion
	protected.Post("/prices", func(c *fiber.Ctx) error {
		p := new(PricePayload)
		c.BodyParser(p)

		cacheKey := fmt.Sprintf("price:%s:%s", p.Store, p.ProductID)
		lastPriceStr, err := rdb.Get(ctxBg, cacheKey).Result()

		if err == redis.Nil {
			rdb.Set(ctxBg, cacheKey, p.Price, 0)
		} else if err == nil {
			var lastPrice float64
			fmt.Sscanf(lastPriceStr, "%f", &lastPrice)

			if EvaluatePriceDrop(lastPrice, p.Price) {
				msg := fmt.Sprintf("🚨 Price Drop Alert!\n%s at %s dropped from %.2f to %.2f THB\nLink: %s",
					p.ProductID, p.Store, lastPrice, p.Price, p.URL)
				go sendLineAlert(lineToken, msg)
			}
			if p.Price != lastPrice {
				rdb.Set(ctxBg, cacheKey, p.Price, 0)
			}
		}

		point := influxdb2.NewPointWithMeasurement("product_price").
			AddTag("store", p.Store).
			AddTag("product_id", p.ProductID).
			AddField("price", p.Price).
			SetTime(time.Now())
		writeAPI.WritePoint(point)

		return c.SendStatus(200)
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
	if token == "" || token == "your_line_notify_token_here" { return }
	apiURL := "https://notify-api.line.me/api/notify"
	data := url.Values{}
	data.Set("message", message)
	req, _ := http.NewRequest("POST", apiURL, strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 10 * time.Second}
	client.Do(req)
}
