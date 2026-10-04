package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"log"
	"math/big"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"omni-tracker-api/internal/config"
	"omni-tracker-api/internal/models"
	"omni-tracker-api/internal/repository"
)

const tokenTTL = 24 * time.Hour

// dummyHash is compared against when the username doesn't exist, so that
// "unknown user" and "wrong password" take the same time (prevents user enumeration by timing).
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcrypt.DefaultCost)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func Register(c *fiber.Ctx) error {
	var payload credentials
	if err := c.BodyParser(&payload); err != nil {
		return jsonError(c, fiber.StatusBadRequest, "Invalid body")
	}
	if !usernameRe.MatchString(payload.Username) {
		return jsonError(c, fiber.StatusBadRequest, "Username must be 3-32 characters: letters, numbers, . _ -")
	}
	// bcrypt only uses the first 72 bytes and Go's implementation rejects longer input.
	if len(payload.Password) < 8 || len(payload.Password) > 72 {
		return jsonError(c, fiber.StatusBadRequest, "Password must be 8-72 characters")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("register: bcrypt failed: %v", err)
		return jsonError(c, fiber.StatusInternalServerError, "Registration failed")
	}
	user := models.User{Username: payload.Username, Password: string(hashed), Role: "user"}

	if err := repository.DB.Create(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return jsonError(c, fiber.StatusConflict, "Username already exists")
		}
		log.Printf("register: db error: %v", err)
		return jsonError(c, fiber.StatusInternalServerError, "Registration failed")
	}
	return c.JSON(fiber.Map{"status": "success", "message": "Registered successfully"})
}

func Login(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var payload credentials
		if err := c.BodyParser(&payload); err != nil {
			return jsonError(c, fiber.StatusBadRequest, "Invalid body")
		}
		if len(payload.Username) > 64 || len(payload.Password) > 72 {
			return jsonError(c, fiber.StatusUnauthorized, "Unauthorized")
		}

		var user models.User
		hash := dummyHash
		found := repository.DB.Where("username = ?", payload.Username).First(&user).Error == nil
		if found {
			hash = []byte(user.Password)
		}
		if bcrypt.CompareHashAndPassword(hash, []byte(payload.Password)) != nil || !found {
			return jsonError(c, fiber.StatusUnauthorized, "Unauthorized")
		}

		now := time.Now()
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(user.ID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
		})
		tokenString, err := token.SignedString([]byte(cfg.JWTSecret))
		if err != nil {
			return jsonError(c, fiber.StatusInternalServerError, "Failed to generate token")
		}
		return c.JSON(fiber.Map{"token": tokenString})
	}
}

// UserMiddleware validates the JWT and loads the user from the DB on every request.
// The role is read from the DB (not from the token), so demoting or deleting a user takes effect immediately.
func UserMiddleware(cfg *config.Config) fiber.Handler {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	return func(c *fiber.Ctx) error {
		tokenString, ok := bearerToken(c)
		if !ok {
			return jsonError(c, fiber.StatusUnauthorized, "Missing or invalid token format")
		}

		claims := &jwt.RegisteredClaims{}
		token, err := parser.ParseWithClaims(tokenString, claims, func(*jwt.Token) (interface{}, error) {
			return []byte(cfg.JWTSecret), nil
		})
		if err != nil || !token.Valid {
			return jsonError(c, fiber.StatusUnauthorized, "Invalid or expired JWT token")
		}
		userID, err := strconv.ParseUint(claims.Subject, 10, 64)
		if err != nil || userID == 0 {
			return jsonError(c, fiber.StatusUnauthorized, "Invalid or expired JWT token")
		}

		var user models.User
		if err := repository.DB.Select("id", "role").First(&user, uint(userID)).Error; err != nil {
			return jsonError(c, fiber.StatusUnauthorized, "User no longer exists")
		}
		c.Locals(localUserID, user.ID)
		c.Locals(localRole, user.Role)
		return c.Next()
	}
}

func AdminMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, ok := c.Locals(localRole).(string)
		if !ok || role != "admin" {
			return jsonError(c, fiber.StatusForbidden, "Forbidden: Admin access required")
		}
		return c.Next()
	}
}

func ScraperMiddleware(cfg *config.Config) fiber.Handler {
	expected := []byte(cfg.APIKey)
	return func(c *fiber.Ctx) error {
		tokenString, ok := bearerToken(c)
		if !ok || subtle.ConstantTimeCompare([]byte(tokenString), expected) != 1 {
			return jsonError(c, fiber.StatusUnauthorized, "Unauthorized Scraper")
		}
		return c.Next()
	}
}

func GetProfile(c *fiber.Ctx) error {
	userID, ok := currentUserID(c)
	if !ok {
		return jsonError(c, fiber.StatusUnauthorized, "Unauthorized")
	}
	var user models.User
	if err := repository.DB.First(&user, userID).Error; err != nil {
		return jsonError(c, fiber.StatusNotFound, "User not found")
	}
	return c.JSON(fiber.Map{
		"username":     user.Username,
		"role":         user.Role,
		"line_linked":  user.LineUserID != "",
		"line_user_id": maskLineID(user.LineUserID),
	})
}

const linkCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I
const linkCodeTTL = 10 * time.Minute

// CreateLineLinkCode issues a short-lived one-time code. The user proves ownership of their
// LINE account by sending "LINK <code>" to the LINE Official Account; the analyst webhook then
// binds the sender's LINE userId to this account. Users can no longer type an arbitrary LINE ID.
func CreateLineLinkCode(c *fiber.Ctx) error {
	userID, ok := currentUserID(c)
	if !ok {
		return jsonError(c, fiber.StatusUnauthorized, "Unauthorized")
	}
	expires := time.Now().Add(linkCodeTTL)
	for attempt := 0; attempt < 3; attempt++ {
		code, err := randomCode(8)
		if err != nil {
			return jsonError(c, fiber.StatusInternalServerError, "Failed to generate code")
		}
		err = repository.DB.Model(&models.User{}).Where("id = ?", userID).
			Updates(map[string]any{"line_link_code": code, "line_link_expires_at": expires}).Error
		if err == nil {
			return c.JSON(fiber.Map{"code": code, "command": "LINK " + code, "expires_at": expires})
		}
		if !errors.Is(err, gorm.ErrDuplicatedKey) {
			log.Printf("link code: db error: %v", err)
			break
		}
	}
	return jsonError(c, fiber.StatusInternalServerError, "Failed to generate code")
}

func UnlinkLine(c *fiber.Ctx) error {
	userID, ok := currentUserID(c)
	if !ok {
		return jsonError(c, fiber.StatusUnauthorized, "Unauthorized")
	}
	if err := repository.DB.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]any{"line_user_id": "", "line_link_code": nil, "line_link_expires_at": nil}).Error; err != nil {
		return jsonError(c, fiber.StatusInternalServerError, "Failed to unlink")
	}
	return c.JSON(fiber.Map{"status": "success"})
}

func randomCode(n int) (string, error) {
	out := make([]byte, n)
	limit := big.NewInt(int64(len(linkCodeAlphabet)))
	for i := range out {
		idx, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		out[i] = linkCodeAlphabet[idx.Int64()]
	}
	return string(out), nil
}

func maskLineID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:4] + "…" + id[len(id)-4:]
}
