package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPPort string
	Env      string
	DB       DBConfig
	Supabase SupabaseConfig
}

type DBConfig struct {
	DSN             string
	Host            string
	Port            string
	User            string
	Password        string
	Name            string
	SSLMode         string
	TimeZone        string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type SupabaseConfig struct {
	URL            string
	PublishableKey string
	AuthTimeout    time.Duration
}

func Load() Config {
	err := loadDotEnv()
	if err != nil {
		panic("Failed to load .env file: " + err.Error())
	}

	return Config{
		HTTPPort: getEnv("HTTP_PORT", "8080"),
		Env:      getEnv("ENV", "development"),
		DB: DBConfig{
			DSN:             getEnv("DB_DSN", ""),
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnv("DB_PORT", "5432"),
			User:            getEnv("DB_USER", "postgres"),
			Password:        getEnv("DB_PASSWORD", "postgres"),
			Name:            getEnv("DB_NAME", "family_app"),
			SSLMode:         getEnv("DB_SSLMODE", "disable"),
			TimeZone:        getEnv("DB_TIMEZONE", "UTC"),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 10),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
		},
		Supabase: SupabaseConfig{
			URL:            getEnv("SUPABASE_URL", ""),
			PublishableKey: getEnv("SUPABASE_PUBLISHABLE_KEY", getEnv("VITE_SUPABASE_PUBLISHABLE_KEY", "")),
			AuthTimeout:    getEnvDuration("SUPABASE_AUTH_TIMEOUT", 5*time.Second),
		},
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func (c DBConfig) GetDSN() string {
	if c.DSN != "" {
		return c.DSN
	}
	return "host=" + c.Host +
		" user=" + c.User +
		" password=" + c.Password +
		" dbname=" + c.Name +
		" port=" + c.Port +
		" sslmode=" + c.SSLMode +
		" TimeZone=" + c.TimeZone
}
