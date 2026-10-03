package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"omni-tracker-api/internal/config"
	"omni-tracker-api/internal/handler"
	"omni-tracker-api/internal/repository"
	"omni-tracker-api/internal/service"
)

func main() {
	cfg := config.LoadConfig()

	// Initialize Storage Layer
	repository.InitDB(cfg.DatabaseURL)
	repository.InitRedis(cfg.RedisURL)
	repository.InitInflux(cfg.InfluxURL, cfg.InfluxToken, cfg.InfluxOrg, cfg.InfluxBucket)

	app := fiber.New()
	app.Use(cors.New())
	
	// Serve static UI
	app.Static("/", "./dashboard")

	apiGroup := app.Group("/api")
	
	// Public Route
	apiGroup.Post("/login", handler.Login(cfg))

	// Protected Routes
	protected := apiGroup.Group("", handler.AuthMiddleware(cfg))
	
	// Product Management
	protected.Get("/products", handler.GetProducts)
	protected.Post("/products", handler.AddProduct)
	protected.Delete("/products/:id", handler.DeleteProduct)
	protected.Get("/history/:product_id", handler.GetHistory(cfg))
	
	// Scraper Dynamic Configurations
	protected.Get("/configs", handler.GetConfigs)
	protected.Post("/configs", handler.UpdateConfig)
	
	// Scraper Worker Endpoints
	protected.Get("/tasks", handler.GetTasks)
	protected.Patch("/products/:id/status", handler.UpdateStatus)
	protected.Post("/prices", handler.PostPrice(cfg))
	protected.Get("/logs", handler.GetLogs)
	protected.Post("/logs", handler.PostLog)

	// Start internal background cron to push tasks to Message Queue
	service.StartTaskScheduler()

	log.Printf("🚀 Omni-Tracker API starting on port %s...", cfg.APIPort)
	log.Fatal(app.Listen(":" + cfg.APIPort))
}
