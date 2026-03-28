package config

import (
	"log"
	"os"
	"time"
)

type JWTConfig struct {
	AccessSecret      string
	RefreshSecret     string
	AccessExpiration  time.Duration
	RefreshExpiration time.Duration
}

type OAuthConfig struct {
	YandexClientID     string
	YandexClientSecret string
	YandexCallbackURL  string
}

var (
	JWT   JWTConfig
	OAuth OAuthConfig
)

func LoadConfig() {
	// JWT
	JWT = JWTConfig{
		AccessSecret:  os.Getenv("JWT_ACCESS_SECRET"),
		RefreshSecret: os.Getenv("JWT_REFRESH_SECRET"),
	}
	JWT.AccessExpiration, _ = time.ParseDuration(os.Getenv("JWT_ACCESS_EXPIRATION"))
	JWT.RefreshExpiration, _ = time.ParseDuration(os.Getenv("JWT_REFRESH_EXPIRATION"))

	// Yandex OAuth
	OAuth = OAuthConfig{
		YandexClientID:     os.Getenv("YANDEX_CLIENT_ID"),
		YandexClientSecret: os.Getenv("YANDEX_CLIENT_SECRET"),
		YandexCallbackURL:  os.Getenv("YANDEX_CALLBACK_URL"),
	}

	log.Println("✅ Конфигурация JWT + Yandex OAuth загружена")
}
