package main

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

// PricePayload struct matches the JSON from Playwright
type PricePayload struct {
	ProductID string  `json:"product_id"`
	Store     string  `json:"store"`
	Price     float64 `json:"price"`
}

func main() {
	app := fiber.New()

	// InfluxDB Connection (Time-Series DB is best for price history)
	client := influxdb2.NewClient("http://influxdb:8086", "super-secret-token")
	writeAPI := client.WriteAPIBlocking("my-org", "market-data")

	// Ingestion Endpoint (High Concurrency)
	app.Post("/api/prices", func(c *fiber.Ctx) error {
		p := new(PricePayload)
		if err := c.BodyParser(p); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
		}

		// Create InfluxDB Point
		point := influxdb2.NewPointWithMeasurement("product_price").
			AddTag("store", p.Store).
			AddTag("product_id", p.ProductID).
			AddField("price", p.Price).
			SetTime(time.Now())

		// Write to DB
		err := writeAPI.WritePoint(context.Background(), point)
		if err != nil {
			log.Printf("DB Write Error: %v", err)
			return c.Status(500).SendString("DB Error")
		}

		// TODO: Trigger LINE Bot API if price < threshold
		// http.Post("http://your-line-bot-api/notify", ...)

		return c.SendStatus(200)
	})

	log.Fatal(app.Listen(":3000"))
}