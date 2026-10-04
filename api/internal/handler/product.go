package handler

import (
	"context"
	"fmt"
	"regexp"
	"github.com/gofiber/fiber/v2"
	"omni-tracker-api/internal/config"
	"omni-tracker-api/internal/models"
	"omni-tracker-api/internal/repository"
)

type AddProductRequest struct {
	ProductID string `json:"product_id"`
	URL       string `json:"url"`
}

func GetProducts(c *fiber.Ctx) error {
	userID, ok := currentUserID(c)
	role, _ := c.Locals(localRole).(string)
	
	var products []models.Product
	if ok {
		repository.DB.Where("user_id = ?", userID).Order("id desc").Find(&products)
	} else if role == "admin" || role == "service" {
		repository.DB.Order("id desc").Find(&products)
	} else {
		return jsonError(c, fiber.StatusUnauthorized, "Unauthorized")
	}
	return c.JSON(products)
}

func AddProduct(c *fiber.Ctx) error {
	var p AddProductRequest
	if err := c.BodyParser(&p); err != nil {
		return jsonError(c, fiber.StatusBadRequest, "invalid payload")
	}

	if !productIDRe.MatchString(p.ProductID) {
		return jsonError(c, fiber.StatusBadRequest, "invalid product id format")
	}

	store, err := StoreFromURL(p.URL)
	if err != nil {
		return jsonError(c, fiber.StatusBadRequest, err.Error())
	}

	userID, ok := currentUserID(c)
	if !ok {
		return jsonError(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	prod := models.Product{
		UserID:    userID,
		ProductID: p.ProductID,
		Store:     store,
		URL:       p.URL,
		Status:    "PENDING",
	}

	if err := repository.DB.Create(&prod).Error; err != nil {
		return jsonError(c, fiber.StatusConflict, "Failed to track product (Duplicate or Database Error)")
	}
	return c.JSON(prod)
}

func DeleteProduct(c *fiber.Ctx) error {
	userID, ok := currentUserID(c)
	role, _ := c.Locals(localRole).(string)
	
	if ok {
		repository.DB.Where("id = ? AND user_id = ?", c.Params("id"), userID).Delete(&models.Product{})
	} else if role == "admin" || role == "service" {
		repository.DB.Delete(&models.Product{}, c.Params("id"))
	} else {
		return jsonError(c, fiber.StatusUnauthorized, "Unauthorized")
	}
	
	cacheKey := fmt.Sprintf("price:product_pk:%s", c.Params("id"))
	repository.RDB.Del(context.Background(), cacheKey)

	return c.SendStatus(200)
}

func GetHistory(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		
		validID := regexp.MustCompile(`^[0-9]+$`)
		if !validID.MatchString(id) {
			return jsonError(c, fiber.StatusBadRequest, "invalid id")
		}

		query := fmt.Sprintf(`from(bucket:"%s") 
			|> range(start: -14d) 
			|> filter(fn: (r) => r._measurement == "product_price" and r.product_pk == "%s")
			|> aggregateWindow(every: 1d, fn: last, createEmpty: false)
			|> yield(name: "last")`, cfg.InfluxBucket, id)
		
		result, err := repository.QueryAPI.Query(context.Background(), query)
		if err != nil {
			return jsonError(c, fiber.StatusInternalServerError, err.Error())
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
			return jsonError(c, fiber.StatusInternalServerError, result.Err().Error())
		}
		return c.JSON(points)
	}
}
