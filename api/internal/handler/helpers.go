package handler

import (
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
)

const (
	localUserID = "userID"
	localRole   = "role"
)

var (
	usernameRe  = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)
	productIDRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)
	allowedStores = map[string]string{ // registrable domain -> store name
		"shopee.co.th": "shopee",
		"lazada.co.th": "lazada",
	}
	validLogLevels = map[string]bool{"INFO": true, "SUCCESS": true, "WARN": true, "ERROR": true}
)

func jsonError(c *fiber.Ctx, status int, msg string) error {
	return c.Status(status).JSON(fiber.Map{"error": msg})
}

func bearerToken(c *fiber.Ctx) (string, bool) {
	h := c.Get(fiber.HeaderAuthorization)
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return h[len(prefix):], true
}

// currentUserID never panics: routes must reject requests without an authenticated user.
func currentUserID(c *fiber.Ctx) (uint, bool) {
	id, ok := c.Locals(localUserID).(uint)
	return id, ok && id > 0
}

// StoreFromURL validates a product URL (SSRF protection) and derives the store from its host.
func StoreFromURL(raw string) (string, error) {
	if len(raw) > 2048 {
		return "", errors.New("url too long")
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("invalid url")
	}
	if u.User != nil {
		return "", errors.New("invalid url")
	}
	if p := u.Port(); p != "" && p != "80" && p != "443" {
		return "", errors.New("domain not allowed")
	}
	host := strings.ToLower(u.Hostname())
	for domain, store := range allowedStores {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return store, nil
		}
	}
	return "", errors.New("domain not allowed")
}
