package config

import (
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	InfluxURL    string
	InfluxToken  string
	InfluxOrg    string
	InfluxBucket string
	APIKey       string
	AdminPass    string
	DatabaseURL  string
	DBMaxOpen    int
	RedisURL     string
	RedisPass    string
	APIPort      string
	JWTSecret    string
	CORSOrigins  string

	// LINE Messaging API (LINE Notify was shut down on 2025-03-31)
	LineChannelToken string

	// Reverse proxy support (used for correct client IPs in the rate limiter)
	ProxyHeader    string
	TrustedProxies []string

	// Scheduler / business rules
	ScrapeIntervalMin   int
	StaleQueueMin       int
	LogRetentionDays    int
	PriceDropThreshold  float64 // percent, e.g. 5 = alert when price drops >= 5%
	AnomalyConfirmCount int64   // accept an "anomalous" price after N consecutive identical observations
}

// Values that ship in docs/examples and must never be used in a real deployment.
var knownWeakSecrets = map[string]bool{
	"admin": true, "admin123": true, "password": true, "changeme": true,
	"super-secret-token": true, "super-secret-jwt-key": true,
	"my-internal-secret-key": true, "your-jwt-secret-key": true,
	"your-internal-secure-key-123": true,
}

func LoadConfig() *Config {
	cfg := &Config{
		InfluxURL:    getEnv("INFLUXDB_URL", "http://influxdb:8086"),
		InfluxToken:  getSecret("INFLUXDB_TOKEN", 16),
		InfluxOrg:    getEnv("INFLUXDB_ORG", "my-org"),
		InfluxBucket: getEnv("INFLUXDB_BUCKET", "market-data"),
		APIKey:       getSecret("API_KEY", 24),
		AdminPass:    getEnv("ADMIN_PASSWORD", ""), // empty => random password generated on first boot
		DatabaseURL:  getRequiredEnv("DATABASE_URL"),
		DBMaxOpen:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
		RedisURL:     getEnv("REDIS_URL", "redis:6379"),
		RedisPass:    getEnv("REDIS_PASSWORD", ""),
		APIPort:      getEnv("API_PORT", "3000"),
		JWTSecret:    getSecret("JWT_SECRET", 32),
		CORSOrigins:  getEnv("CORS_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"),

		LineChannelToken: getEnv("LINE_CHANNEL_ACCESS_TOKEN", ""),

		ProxyHeader:    getEnv("PROXY_HEADER", ""),
		TrustedProxies: splitCSV(getEnv("TRUSTED_PROXIES", "")),

		ScrapeIntervalMin:   getEnvInt("SCRAPE_INTERVAL_MINUTES", 10),
		StaleQueueMin:       getEnvInt("STALE_QUEUE_MINUTES", 30),
		LogRetentionDays:    getEnvInt("LOG_RETENTION_DAYS", 7),
		PriceDropThreshold:  getEnvFloat("PRICE_DROP_THRESHOLD_PERCENT", 5),
		AnomalyConfirmCount: int64(getEnvInt("ANOMALY_CONFIRM_COUNT", 3)),
	}

	if cfg.AdminPass != "" && (len(cfg.AdminPass) < 12 || knownWeakSecrets[strings.ToLower(cfg.AdminPass)]) {
		log.Fatal("Fatal: ADMIN_PASSWORD is too weak (min 12 chars, not a default value). Leave it empty to auto-generate one.")
	}
	if cfg.ProxyHeader != "" && len(cfg.TrustedProxies) == 0 {
		log.Fatal("Fatal: PROXY_HEADER is set but TRUSTED_PROXIES is empty (clients could spoof their IP)")
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
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

// getSecret loads a required secret and refuses short or well-known placeholder values.
func getSecret(key string, minLen int) string {
	v := getRequiredEnv(key)
	if len(v) < minLen || knownWeakSecrets[strings.ToLower(v)] {
		log.Fatalf("Fatal: %s is too weak (min %d chars and must not be a documented default)", key, minLen)
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
		log.Printf("Warning: invalid %s=%q, using default %d", key, v, fallback)
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			return f
		}
		log.Printf("Warning: invalid %s=%q, using default %v", key, v, fallback)
	}
	return fallback
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
