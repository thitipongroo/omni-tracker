package main

import (
	"context"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"time"
	"omni-tracker-api/internal/config"
	"omni-tracker-api/internal/handler"
	"omni-tracker-api/internal/repository"
	"omni-tracker-api/internal/service"
)

func main() {
	cfg := config.LoadConfig()

	// Initialize Storage Layer
	repository.InitDB(cfg.DatabaseURL, cfg.AdminPass, cfg.DBMaxOpen)
	repository.InitRedis(cfg.RedisURL, cfg.RedisPass)
	repository.InitInflux(cfg.InfluxURL, cfg.InfluxToken, cfg.InfluxOrg, cfg.InfluxBucket)

	app := fiber.New()
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSOrigins,
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))
	
	// Serve static UI from dist
	app.Static("/", "./dashboard/dist")

	// SPA Fallback for React Router (Prevents 404 on page refresh)
	app.Use(func(c *fiber.Ctx) error {
		if err := c.Next(); err != nil {
			if e, ok := err.(*fiber.Error); ok && e.Code == fiber.StatusNotFound {
				// Don't fallback for missing API routes
				if len(c.Path()) >= 4 && c.Path()[:4] == "/api" {
					return err
				}
				return c.SendFile("./dashboard/dist/index.html")
			}
			return err
		}
		return nil
	})

	apiGroup := app.Group("/api")
	
	// Public Route with Rate Limiter
	apiGroup.Post("/login", limiter.New(limiter.Config{
		Max:        5,
		Expiration: 1 * time.Minute,
	}), handler.Login(cfg))
	
	apiGroup.Post("/register", limiter.New(limiter.Config{
		Max:        5,
		Expiration: 1 * time.Minute,
	}), handler.Register)

	// Protected User Routes (Prefix with /v1 to avoid middleware leaking to sibling groups)
	userRoutes := apiGroup.Group("/v1", handler.UserMiddleware(cfg))
	
	// User Profile
	userRoutes.Get("/profile", handler.GetProfile)
	userRoutes.Post("/profile/line", handler.CreateLineLinkCode)
	userRoutes.Delete("/profile/line", handler.UnlinkLine)
	
	// Product Management
	userRoutes.Get("/products", handler.GetProducts)
	userRoutes.Post("/products", limiter.New(limiter.Config{
		Max:        20,
		Expiration: 1 * time.Minute,
	}), handler.AddProduct)
	userRoutes.Delete("/products/:id", handler.DeleteProduct)
	userRoutes.Get("/history/:id", handler.GetHistory(cfg))
	userRoutes.Get("/logs", handler.GetLogs)
	
	// Protected Admin Routes
	adminRoutes := userRoutes.Group("", handler.AdminMiddleware())
	adminRoutes.Get("/configs", handler.GetConfigs)
	adminRoutes.Post("/configs", handler.UpdateConfig)
	
	// Scraper Internal Routes (Prefix with /internal)
	scraperRoutes := apiGroup.Group("/internal", handler.ScraperMiddleware(cfg))
	scraperRoutes.Patch("/products/:id/status", handler.UpdateStatus)
	scraperRoutes.Post("/prices", handler.PostPrice(cfg))
	scraperRoutes.Post("/logs", handler.PostLog)

	// Start internal background cron to push tasks to Message Queue
	service.StartTaskScheduler(context.Background(), cfg)

	log.Printf("🚀 Omni-Tracker API starting on port %s...", cfg.APIPort)
	log.Fatal(app.Listen(":" + cfg.APIPort))
}
