package models

import "time"

type StoreConfig struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Store    string `gorm:"uniqueIndex;not null" json:"store"`
	Selector string `gorm:"not null" json:"selector"`
}

type Product struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	ProductID     string     `gorm:"not null" json:"product_id"`
	Store         string     `gorm:"not null" json:"store"`
	URL           string     `gorm:"not null" json:"url"`
	IsActive      bool       `gorm:"default:true" json:"is_active"`
	Status        string     `gorm:"default:'PENDING'" json:"status"`
	LastScrapedAt *time.Time `json:"last_scraped_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

type TaskResponse struct {
	Product
	Selector string `json:"selector"`
}

type PricePayload struct {
	ProductID string  `json:"product_id"`
	Store     string  `json:"store"`
	Price     float64 `json:"price"`
	URL       string  `json:"url"`
}

type Point struct {
	Time  time.Time `json:"time"`
	Price float64   `json:"price"`
}
