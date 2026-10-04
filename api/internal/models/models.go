package models

import "time"

type User struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	Username          string     `gorm:"uniqueIndex;not null" json:"username"`
	Password          string     `gorm:"not null" json:"-"`
	Role              string     `gorm:"default:'user'" json:"role"`
	LineUserID        string     `gorm:"index" json:"line_user_id"`
	LineLinkCode      *string    `gorm:"uniqueIndex;size:16" json:"-"`
	LineLinkExpiresAt *time.Time `json:"-"`
}

type StoreConfig struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Store    string `gorm:"uniqueIndex;not null" json:"store"`
	Selector string `gorm:"not null" json:"selector"`
}

type Product struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	UserID        uint       `gorm:"uniqueIndex:idx_user_product_store" json:"user_id"`
	ProductID     string     `gorm:"uniqueIndex:idx_user_product_store;not null" json:"product_id"`
	Store         string     `gorm:"uniqueIndex:idx_user_product_store;not null" json:"store"`
	URL           string     `gorm:"not null" json:"url"`
	IsActive      bool       `gorm:"default:true" json:"is_active"`
	Status        string     `gorm:"default:'PENDING'" json:"status"`
	LastScrapedAt *time.Time `json:"last_scraped_at"`
	QueuedAt      *time.Time `json:"-"`
	CreatedAt     time.Time  `json:"created_at"`
}

type ScrapeLog struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// ProductPK references products.id. All tenant scoping uses this, never the free-text ProductID.
	ProductPK uint      `gorm:"index:idx_logs_pk_created,priority:1" json:"product_pk"`
	ProductID string    `gorm:"not null" json:"product_id"` // display only
	Store     string    `json:"store"`
	Message   string    `json:"message"`
	Level     string    `json:"level"`
	CreatedAt time.Time `gorm:"index:idx_logs_pk_created,priority:2,sort:desc;index" json:"created_at"`
}

// ScrapeTask is the minimal payload pushed to the Redis queue for scraper workers.
type ScrapeTask struct {
	ID        uint   `json:"id"`
	ProductID string `json:"product_id"`
	Store     string `json:"store"`
	URL       string `json:"url"`
	Selector  string `json:"selector"`
}

// PricePayload is sent by scraper workers. Everything else is looked up from the DB by ID.
type PricePayload struct {
	ID    uint    `json:"id"`
	Price float64 `json:"price"`
}

type Point struct {
	Time  time.Time `json:"time"`
	Price float64   `json:"price"`
}

const (
	StatusPending = "PENDING"
	StatusQueued  = "QUEUED"
	StatusSuccess = "SUCCESS"
	StatusFailed  = "FAILED"
	StatusAnomaly = "ANOMALY"
)
