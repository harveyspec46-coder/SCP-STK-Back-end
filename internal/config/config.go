package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port               string
	Env                string
	DatabaseURL        string
	SupabaseURL        string
	SupabaseAnonKey    string
	SupabaseServiceKey string
	SupabaseJWTSecret  string
	MSClientID         string
	MSClientSecret     string
	MSTenantID         string
	MakeWebhookURL     string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, reading from environment")
	}
	return &Config{
		Port:               getEnv("PORT", "8080"),
		Env:                getEnv("ENV", "development"),
		DatabaseURL:        mustEnv("DATABASE_URL"),
		SupabaseURL:        mustEnv("SUPABASE_URL"),
		SupabaseAnonKey:    mustEnv("SUPABASE_ANON_KEY"),
		SupabaseServiceKey: mustEnv("SUPABASE_SERVICE_KEY"),
		SupabaseJWTSecret:  mustEnv("SUPABASE_JWT_SECRET"),
		MSClientID:         getEnv("MS_CLIENT_ID", ""),
		MSClientSecret:     getEnv("MS_CLIENT_SECRET", ""),
		MSTenantID:         getEnv("MS_TENANT_ID", ""),
		MakeWebhookURL:     getEnv("MAKE_WEBHOOK_URL", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %s is not set", key)
	}
	return v
}
