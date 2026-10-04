package config

import (
	"log"
	"os"
)

type Config struct {
	InfluxURL    string
	InfluxToken  string
	InfluxOrg    string
	InfluxBucket string
	APIKey       string
	AdminPass    string
	LineToken    string
	DatabaseURL  string
	RedisURL     string
	APIPort      string
	JWTSecret    string
	CORSOrigins  string
}

func LoadConfig() *Config {
	return &Config{
		InfluxURL:    getEnv("INFLUXDB_URL", "http://influxdb:8086"),
		InfluxToken:  getEnv("INFLUXDB_TOKEN", "super-secret-token"),
		InfluxOrg:    getEnv("INFLUXDB_ORG", "my-org"),
		InfluxBucket: getEnv("INFLUXDB_BUCKET", "market-data"),
		APIKey:       getRequiredEnv("API_KEY"),
		AdminPass:    getEnv("ADMIN_PASSWORD", "admin123"),
		LineToken:    getEnv("LINE_NOTIFY_TOKEN", ""),
		DatabaseURL:  getEnv("DATABASE_URL", "host=postgres user=admin password=admin dbname=omnitracker port=5432 sslmode=disable"),
		RedisURL:     getEnv("REDIS_URL", "redis:6379"),
		APIPort:      getEnv("API_PORT", "3000"),
		JWTSecret:    getRequiredEnv("JWT_SECRET"),
		CORSOrigins:  getEnv("CORS_ORIGINS", "http://localhost:3000, http://127.0.0.1:3000"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getRequiredEnv(key string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	log.Fatalf("Fatal: Environment variable %s is required but not set", key)
	return ""
}
