package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Connect() *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
	)

	var db *gorm.DB
	var err error

	// Повторяем попытки подключения до 30 раз (примерно 60 секунд)
	for i := 0; i < 30; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			// Проверяем реальное соединение
			sqlDB, err := db.DB()
			if err == nil {
				err = sqlDB.Ping()
				if err == nil {
					log.Println("✅ Успешное подключение к PostgreSQL")
					// Настройка пула соединений (рекомендуется)
					sqlDB.SetMaxIdleConns(10)
					sqlDB.SetMaxOpenConns(100)
					sqlDB.SetConnMaxLifetime(time.Hour)
					return db
				}
			}
		}

		log.Printf("Попытка %d/30: Не удалось подключиться к БД: %v. Ждём 2 секунды...", i+1, err)
		time.Sleep(2 * time.Second)
	}

	log.Fatal("❌ Не удалось подключиться к базе данных после 30 попыток")
	return nil
}
