package repository

import (
	"log"
	"time"

	"omni-tracker-api/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(dsn string) {
	var err error
	for i := 0; i < 5; i++ {
		DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		log.Println("Waiting for PostgreSQL to start...")
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatal("Failed to connect to DB:", err)
	}
	DB.AutoMigrate(&models.Product{}, &models.StoreConfig{})

	seedDefaults()
}

func seedDefaults() {
	var count int64
	DB.Model(&models.StoreConfig{}).Count(&count)
	if count == 0 {
		DB.Create(&models.StoreConfig{Store: "shopee", Selector: ".product-price"})
		DB.Create(&models.StoreConfig{Store: "lazada", Selector: ".pdp-price"})
	}
}
