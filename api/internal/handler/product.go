package handler

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"github.com/gofiber/fiber/v2"
	"omni-tracker-api/internal/config"
	"omni-tracker-api/internal/models"
	"omni-tracker-api/internal/repository"
)

func GetProducts(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(uint)
	var products []models.Product
	if ok {
		repository.DB.Where("user_id = ?", userID).Order("id desc").Find(&products)
	} else {
		repository.DB.Order("id desc").Find(&products) // For Admin/Service
	}
	return c.JSON(products)
}

func AddProduct(c *fiber.Ctx) error {
	p := new(models.Product)
	if err := c.BodyParser(p); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}

	// SSRF Protection: Validate URL scheme and host whitelist
	parsedURL, err := url.ParseRequestURI(p.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return c.Status(400).JSON(fiber.Map{"error": "invalid url"})
	}
	validHosts := []string{"shopee.co.th", "lazada.co.th"}
	isValidHost := false
	for _, host := range validHosts {
		if strings.HasSuffix(parsedURL.Host, host) {
			isValidHost = true
			break
		}
	}
	if !isValidHost {
		return c.Status(400).JSON(fiber.Map{"error": "domain not allowed"})
	}

	if userID, ok := c.Locals("userID").(uint); ok {
		p.UserID = userID
	}
	p.Status = "PENDING"
	repository.DB.Create(p)
	return c.JSON(p)
}

func DeleteProduct(c *fiber.Ctx) error {
	if userID, ok := c.Locals("userID").(uint); ok {
		repository.DB.Where("id = ? AND user_id = ?", c.Params("id"), userID).Delete(&models.Product{})
	} else {
		repository.DB.Delete(&models.Product{}, c.Params("id"))
	}
	return c.SendStatus(200)
}

func GetHistory(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		pid := c.Params("product_id")

		// Prevent InfluxQL Injection
		validID := regexp.MustCompile(`^[a-zA-Z0-9\-_]+$`)
		if !validID.MatchString(pid) {
			return c.Status(400).JSON(fiber.Map{"error": "invalid product id format"})
		}

		query := fmt.Sprintf(`from(bucket:"%s") 
			|> range(start: -14d) 
			|> filter(fn: (r) => r._measurement == "product_price" and r.product_id == "%s")
			|> aggregateWindow(every: 1d, fn: last, createEmpty: false)
			|> yield(name: "last")`, cfg.InfluxBucket, pid)
		
		result, err := repository.QueryAPI.Query(repository.Ctx, query)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		
		var points []models.Point
		for result.Next() {
			if val, ok := result.Record().Value().(float64); ok {
				points = append(points, models.Point{
					Time:  result.Record().Time(),
					Price: val,
				})
			}
		}
		if result.Err() != nil {
			return c.Status(500).SendString(result.Err().Error())
		}
		return c.JSON(points)
	}
}
