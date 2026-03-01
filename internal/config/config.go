package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"family-app-go/pkg/logger"
)

type Config struct {
	HTTPPort           string
	Env                string
	OfflineSyncEnabled bool
	TopCategories      TopCategoriesConfig
	DB                 DBConfig
	Supabase           SupabaseConfig
}

type TopCategoriesConfig struct {
	Enabled       bool
	LookbackDays  int
	DBReadLimit   int
	MinRecords    int
	ResponseCount int
	CacheTTL      time.Duration
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
	SkipAuth       bool
	MockUserID     string
	MockUserEmail  string
	MockUserName   string
	MockUserAvatar string
}

func Load(log logger.Logger) (Config, error) {
	err := loadDotEnv(log)
	if err != nil {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	return Config{
		HTTPPort:           getEnv("HTTP_PORT", "8080"),
		Env:                getEnv("ENV", "development"),
		OfflineSyncEnabled: getEnvBool("OFFLINE_SYNC_ENABLED", true),
		TopCategories: TopCategoriesConfig{
			Enabled:       getEnvBool("TOP_CATEGORIES_ENABLED", true),
			LookbackDays:  getEnvInt("TOP_CATEGORIES_LOOKBACK_DAYS", 30),
			DBReadLimit:   getEnvInt("TOP_CATEGORIES_DB_READ_LIMIT", 1000),
			MinRecords:    getEnvInt("TOP_CATEGORIES_MIN_RECORDS", 10),
			ResponseCount: getEnvInt("TOP_CATEGORIES_RESPONSE_COUNT", 5),
			CacheTTL:      getEnvDuration("TOP_CATEGORIES_CACHE_TTL", time.Minute),
		},
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
			SkipAuth:       getEnvBool("AUTH_SKIP", false),
			MockUserID:     getEnv("AUTH_MOCK_USER_ID", "00000000-0000-0000-0000-000000000001"),
			MockUserEmail:  getEnv("AUTH_MOCK_USER_EMAIL", ""),
			MockUserName:   getEnv("AUTH_MOCK_USER_NAME", ""),
			MockUserAvatar: getEnv("AUTH_MOCK_USER_AVATAR_URL", ""),
		},
	}, nil
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

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
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
