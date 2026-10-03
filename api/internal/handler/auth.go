package handler

import (
	"github.com/gofiber/fiber/v2"
	"omni-tracker-api/internal/config"
)

func Login(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var payload struct {
			Password string `json:"password"`
		}
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(400).SendString("Invalid body")
		}
		if payload.Password == cfg.AdminPass {
			return c.JSON(fiber.Map{"token": cfg.APIKey})
		}
		return c.Status(401).SendString("Unauthorized")
	}
}

func AuthMiddleware(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Get("Authorization")
		if token != "Bearer "+cfg.APIKey {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		return c.Next()
	}
}
