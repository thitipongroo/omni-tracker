package handler

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"omni-tracker-api/internal/config"
	"omni-tracker-api/internal/models"
	"omni-tracker-api/internal/repository"
)

func Register(c *fiber.Ctx) error {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(400).SendString("Invalid body")
	}

	if len(payload.Username) < 3 || len(payload.Password) < 6 {
		return c.Status(400).JSON(fiber.Map{"error": "Username must be >= 3 and password >= 6 characters"})
	}

	hashed, _ := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	user := models.User{
		Username: payload.Username,
		Password: string(hashed),
		Role:     "user",
	}

	if err := repository.DB.Create(&user).Error; err != nil {
		return c.Status(409).JSON(fiber.Map{"error": "Username already exists"})
	}

	return c.JSON(fiber.Map{"status": "success", "message": "Registered successfully"})
}

func Login(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var payload struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(400).SendString("Invalid body")
		}
		
		var user models.User
		if err := repository.DB.Where("username = ?", payload.Username).First(&user).Error; err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(payload.Password)); err == nil {
			// Generate JWT Token
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": user.ID,
				"role":    user.Role,
				"exp":     time.Now().Add(time.Hour * 24).Unix(),
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

func UserMiddleware(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing or invalid token format"})
		}
		
		tokenString := authHeader[7:]

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

		claims := token.Claims.(jwt.MapClaims)
		if userID, ok := claims["user_id"].(float64); ok {
			c.Locals("userID", uint(userID))
		}
		if role, ok := claims["role"].(string); ok {
			c.Locals("role", role)
		}

		return c.Next()
	}
}

func AdminMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, ok := c.Locals("role").(string)
		if !ok || role != "admin" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: Admin access required"})
		}
		return c.Next()
	}
}

func ScraperMiddleware(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing or invalid token format"})
		}
		
		tokenString := authHeader[7:]
		
		if tokenString == cfg.APIKey {
			return c.Next()
		}
		
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized Scraper"})
	}
}

func GetProfile(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uint)
	var user models.User
	repository.DB.First(&user, userID)
	return c.JSON(fiber.Map{
		"username": user.Username,
		"line_user_id": user.LineUserID,
	})
}

func UpdateLineID(c *fiber.Ctx) error {
	var payload struct {
		LineUserID string `json:"line_user_id"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid payload"})
	}
	userID := c.Locals("userID").(uint)
	repository.DB.Model(&models.User{}).Where("id = ?", userID).Update("line_user_id", payload.LineUserID)
	return c.JSON(fiber.Map{"status": "success", "line_user_id": payload.LineUserID})
}
