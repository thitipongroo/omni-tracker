package repository

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"omni-tracker-api/internal/models"
)

var DB *gorm.DB

func InitDB(dsn string, adminPass string, maxOpen int) {
	var err error
	gcfg := &gorm.Config{
		TranslateError: true, // lets us detect gorm.ErrDuplicatedKey
		PrepareStmt:    true,
		Logger:         logger.Default.LogMode(logger.Warn),
	}
	for i := 0; i < 10; i++ {
		DB, err = gorm.Open(postgres.Open(dsn), gcfg)
		if err == nil {
			break
		}
		log.Printf("Waiting for PostgreSQL to start... (%d/10)", i+1)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatal("Failed to connect to DB:", err)
	}
	if err := DB.AutoMigrate(&models.User{}, &models.Product{}, &models.StoreConfig{}, &models.ScrapeLog{}); err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}

	// Supports the scheduler query (active products ordered by last scrape time).
	if err := DB.Exec(`CREATE INDEX IF NOT EXISTS idx_products_schedulable
		ON products (last_scraped_at) WHERE is_active`).Error; err != nil {
		log.Printf("Warning: could not create idx_products_schedulable: %v", err)
	}

	sqlDB, err := DB.DB()
	if err == nil {
		// Keep well below Postgres' default max_connections=100 (analyst service also holds a pool).
		sqlDB.SetMaxOpenConns(maxOpen)
		sqlDB.SetMaxIdleConns(maxOpen / 2)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	seedDefaults(adminPass)
}

func CloseDB() {
	if DB == nil {
		return
	}
	if sqlDB, err := DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
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
		generated := false
		if adminPass == "" {
			adminPass = randomPassword()
			generated = true
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("Failed to hash admin password: %v", err)
		}
		if err := DB.Create(&models.User{Username: "admin", Password: string(hashed), Role: "admin"}).Error; err != nil {
			log.Fatalf("Failed to seed admin user: %v", err)
		}
		if generated {
			// Printed exactly once, on first boot only.
			log.Printf("🔐 Seeded admin user. Username: admin  Password: %s  (store it now, it will not be shown again)", adminPass)
		}
	}
}

func randomPassword() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("crypto/rand failed: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
