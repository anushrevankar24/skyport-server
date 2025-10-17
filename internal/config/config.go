package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	WebAppURL   string
	Domain      string
	TunnelType  string // "port" or "subdomain"
	BasePort    int    // Starting port for port-based tunnels
}

func Load() *Config {
	return &Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://anush:anush24@localhost/skyport?sslmode=disable"),
		JWTSecret:   getEnv("JWT_SECRET", "your-super-secret-jwt-key-change-this-in-production"),
		WebAppURL:   getEnv("WEB_APP_URL", "http://localhost:3000"),
		Domain:      getEnv("SKYPORT_DOMAIN", "localhost:8080"), // localhost:8080 for local, yourdomain.com for production
		TunnelType:  getEnv("SKYPORT_TUNNEL_TYPE", "subdomain"), // Always subdomain-based
		BasePort:    getEnvInt("SKYPORT_BASE_PORT", 8081),       // Not used for subdomain mode
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return fallback
}
