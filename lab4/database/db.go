package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"awesomeProject/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Connect подключается к PostgreSQL с retry-стратегией (важно при запуске в docker-compose).
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

	for i := 0; i < 30; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			sqlDB, dbErr := db.DB()
			if dbErr == nil {
				if pingErr := sqlDB.Ping(); pingErr == nil {
					log.Println("Успешное подключение к PostgreSQL")
					sqlDB.SetMaxIdleConns(10)
					sqlDB.SetMaxOpenConns(100)
					sqlDB.SetConnMaxLifetime(time.Hour)
					return db
				}
			}
		}
		log.Printf("Попытка %d/30: Не удалось подключиться к БД: %v. Жду 2 секунды...", i+1, err)
		time.Sleep(2 * time.Second)
	}

	log.Fatal("Не удалось подключиться к базе данных после 30 попыток")
	return nil
}

// Migrate выполняет авто-миграцию всех моделей.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.Token{},
		&models.PasswordResetToken{},
		&models.Item{},
	)
}
