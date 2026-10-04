package repository

import (
	"log"
	"time"

	"omni-tracker-api/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(dsn string, adminPass string) {
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
	if err := DB.AutoMigrate(&models.User{}, &models.Product{}, &models.StoreConfig{}, &models.ScrapeLog{}); err != nil {
		log.Printf("Warning: AutoMigrate failed: %v", err)
	}

	sqlDB, err := DB.DB()
	if err == nil {
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	seedDefaults(adminPass)
}

func seedDefaults(adminPass string) {
	var count int64
	DB.Model(&models.StoreConfig{}).Count(&count)
	if count == 0 {
		DB.Create(&models.StoreConfig{Store: "shopee", Selector: ".product-price"})
		DB.Create(&models.StoreConfig{Store: "lazada", Selector: ".pdp-price"})
	}
	
	var userCount int64
	DB.Model(&models.User{}).Count(&userCount)
	if userCount == 0 {
		hashed, _ := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
		DB.Create(&models.User{Username: "admin", Password: string(hashed), Role: "admin"})
	}
}
