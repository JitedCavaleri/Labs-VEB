package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config — централизованное хранилище настроек приложения.
type Config struct {
	// Окружение приложения: "development", "local" или "production".
	// От него зависит, инициализируется ли Swagger UI (см. main.go).
	AppEnv string

	// JWT
	JWTAccessSecret      string
	JWTRefreshSecret     string
	JWTAccessExpiration  time.Duration
	JWTRefreshExpiration time.Duration

	// OAuth (Yandex / VK)
	YandexClientID     string
	YandexClientSecret string
	YandexCallbackURL  string

	VKClientID     string
	VKClientSecret string
	VKCallbackURL  string

	// Cookies
	CookieDomain string
	CookieSecure bool // true только в проде по HTTPS

	// App
	AppPort string
}

// IsProduction возвращает true, если приложение работает в боевом окружении.
// Используется, чтобы запретить отдачу /api/docs в проде (лаба 4, требование о безопасности документации).
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.AppEnv, "production")
}

// LoadConfig читает .env (через docker-compose env_file либо godotenv) в Config.
func LoadConfig() *Config {
	cfg := &Config{
		AppEnv: getEnv("APP_ENV", "development"),

		JWTAccessSecret:      getEnv("JWT_ACCESS_SECRET", "dev_access_secret_change_me"),
		JWTRefreshSecret:     getEnv("JWT_REFRESH_SECRET", "dev_refresh_secret_change_me"),
		JWTAccessExpiration:  parseDuration(getEnv("JWT_ACCESS_EXPIRATION", "15m")),
		JWTRefreshExpiration: parseDuration(getEnv("JWT_REFRESH_EXPIRATION", "168h")), // 7 дней

		YandexClientID:     os.Getenv("YANDEX_CLIENT_ID"),
		YandexClientSecret: os.Getenv("YANDEX_CLIENT_SECRET"),
		YandexCallbackURL:  getEnv("YANDEX_CALLBACK_URL", "http://localhost:4200/auth/oauth/yandex/callback"),

		VKClientID:     os.Getenv("VK_CLIENT_ID"),
		VKClientSecret: os.Getenv("VK_CLIENT_SECRET"),
		VKCallbackURL:  getEnv("VK_CALLBACK_URL", "http://localhost:4200/auth/oauth/vk/callback"),

		CookieDomain: getEnv("COOKIE_DOMAIN", ""),
		CookieSecure: parseBool(getEnv("COOKIE_SECURE", "false")),

		AppPort: getEnv("PORT", "4200"),
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseDuration принимает строки вида "15m", "7d", "24h".
func parseDuration(s string) time.Duration {
	// Поддержка "Nd" — дни. Go-шный time.ParseDuration их не понимает.
	if len(s) > 1 && s[len(s)-1] == 'd' {
		days, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			log.Printf("Не удалось распарсить duration %q, использую 15m по умолчанию", s)
			return 15 * time.Minute
		}
		return time.Duration(days) * 24 * time.Hour
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Printf("Не удалось распарсить duration %q, использую 15m по умолчанию", s)
		return 15 * time.Minute
	}
	return d
}

func parseBool(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}
