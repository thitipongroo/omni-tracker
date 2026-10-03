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
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

// PricePayload matches the JSON from Playwright
type PricePayload struct {
	ProductID string  `json:"product_id"`
	Store     string  `json:"store"`
	Price     float64 `json:"price"`
	URL       string  `json:"url"`
}

// ScrapeTask represents a URL to be scraped
type ScrapeTask struct {
	ProductID string `json:"product_id"`
	Store     string `json:"store"`
	URL       string `json:"url"`
}

func main() {
	app := fiber.New()

	influxURL := getEnv("INFLUXDB_URL", "http://influxdb:8086")
	influxToken := getEnv("INFLUXDB_TOKEN", "super-secret-token")
	influxOrg := getEnv("INFLUXDB_ORG", "my-org")
	influxBucket := getEnv("INFLUXDB_BUCKET", "market-data")
	apiKey := getEnv("API_KEY", "my-internal-secret-key")
	lineToken := getEnv("LINE_NOTIFY_TOKEN", "")

	// InfluxDB Connection
	client := influxdb2.NewClient(influxURL, influxToken)
	writeAPI := client.WriteAPI(influxOrg, influxBucket) // Changed to Async write
	queryAPI := client.QueryAPI(influxOrg)

	// Auth Middleware to secure API
	app.Use(func(c *fiber.Ctx) error {
		if strings.HasPrefix(c.Path(), "/api/") {
			token := c.Get("Authorization")
			if token != "Bearer "+apiKey {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
			}
		}
		return c.Next()
	})

	// 2. Task Distribution Endpoint
	app.Get("/api/tasks", func(c *fiber.Ctx) error {
		// In a real production app, these would come from a database like PostgreSQL or MongoDB
		tasks := []ScrapeTask{
			{
				ProductID: "iphone-15-pro",
				Store:     "shopee",
				URL:       "https://example-ecommerce.com/product/123",
			},
			{
				ProductID: "nintendo-switch-oled",
				Store:     "lazada",
				URL:       "https://example-ecommerce.com/product/456",
			},
		}
		return c.JSON(tasks)
	})

	// 1 & 3. Ingestion Endpoint with Alert Engine
	app.Post("/api/prices", func(c *fiber.Ctx) error {
		p := new(PricePayload)
		if err := c.BodyParser(p); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
		}

		if p.Price <= 0 || p.ProductID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid data format"})
		}

		ctx := context.Background()

		// Query the last known price to detect price drops
		query := fmt.Sprintf(`from(bucket:"%s") 
			|> range(start: -30d) 
			|> filter(fn: (r) => r._measurement == "product_price" and r.product_id == "%s" and r.store == "%s")
			|> last()`, influxBucket, p.ProductID, p.Store)

		result, err := queryAPI.Query(ctx, query)
		var lastPrice float64
		if err == nil && result.Next() {
			if val, ok := result.Record().Value().(float64); ok {
				lastPrice = val
			}
		}

		// Alerting Logic: If new price is lower than the last known price
		if lastPrice > 0 && p.Price < lastPrice {
			msg := fmt.Sprintf("🚨 Price Drop Alert!\n%s at %s dropped from %.2f to %.2f THB\nLink: %s",
				p.ProductID, p.Store, lastPrice, p.Price, p.URL)

			// Trigger LINE notification asynchronously
			go sendLineAlert(lineToken, msg)
		}

		// Create InfluxDB Point
		point := influxdb2.NewPointWithMeasurement("product_price").
			AddTag("store", p.Store).
			AddTag("product_id", p.ProductID).
			AddField("price", p.Price).
			SetTime(time.Now())

		// Write to DB asynchronously for high concurrency
		writeAPI.WritePoint(point)

		return c.SendStatus(fiber.StatusOK)
	})

	// Add health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("OK")
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

// sendLineAlert sends a notification to LINE Notify API
func sendLineAlert(token, message string) {
	if token == "" || token == "your_line_notify_token_here" {
		log.Println("Skipping LINE alert: Token not set")
		return
	}

	apiURL := "https://notify-api.line.me/api/notify"
	data := url.Values{}
	data.Set("message", message)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		log.Printf("Failed to create LINE request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to send LINE alert: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("LINE API Error: %d", resp.StatusCode)
	} else {
		log.Println("LINE alert sent successfully")
	}
}
