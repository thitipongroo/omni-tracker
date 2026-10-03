package handler

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"omni-tracker-api/internal/config"
	"omni-tracker-api/internal/models"
	"omni-tracker-api/internal/repository"
)

func GetProducts(c *fiber.Ctx) error {
	var products []models.Product
	repository.DB.Order("id desc").Find(&products)
	return c.JSON(products)
}

func AddProduct(c *fiber.Ctx) error {
	p := new(models.Product)
	if err := c.BodyParser(p); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}
	p.Status = "PENDING"
	repository.DB.Create(p)
	return c.JSON(p)
}

func DeleteProduct(c *fiber.Ctx) error {
	repository.DB.Delete(&models.Product{}, c.Params("id"))
	return c.SendStatus(200)
}

func GetHistory(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		pid := c.Params("product_id")
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
