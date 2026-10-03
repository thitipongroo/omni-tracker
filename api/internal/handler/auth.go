package handler

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
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
			// Generate JWT Token
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"role": "admin",
				"exp":  time.Now().Add(time.Hour * 24).Unix(), // 24 hours expiry
			})

			tokenString, err := token.SignedString([]byte(cfg.JWTSecret))
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
			}
			return c.JSON(fiber.Map{"token": tokenString})
		}
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}
}

func AuthMiddleware(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing or invalid token format"})
		}
		
		tokenString := authHeader[7:]
		
		// Internal Service-to-Service auth (Scraper Bot uses static API_KEY)
		if tokenString == cfg.APIKey {
			return c.Next()
		}

		// Verify JWT for Dashboard Users
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return []byte(cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid or expired JWT token"})
		}

		return c.Next()
	}
}
