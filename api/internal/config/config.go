package config

import "os"

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
}

func LoadConfig() *Config {
	return &Config{
		InfluxURL:    getEnv("INFLUXDB_URL", "http://influxdb:8086"),
		InfluxToken:  getEnv("INFLUXDB_TOKEN", "super-secret-token"),
		InfluxOrg:    getEnv("INFLUXDB_ORG", "my-org"),
		InfluxBucket: getEnv("INFLUXDB_BUCKET", "market-data"),
		APIKey:       getEnv("API_KEY", "my-internal-secret-key"),
		AdminPass:    getEnv("ADMIN_PASSWORD", "admin123"),
		LineToken:    getEnv("LINE_NOTIFY_TOKEN", ""),
		DatabaseURL:  getEnv("DATABASE_URL", "host=postgres user=admin password=admin dbname=omnitracker port=5432 sslmode=disable"),
		RedisURL:     getEnv("REDIS_URL", "redis:6379"),
		APIPort:      getEnv("API_PORT", "3000"),
		JWTSecret:    getEnv("JWT_SECRET", "default-fallback-secret-key-1234"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
