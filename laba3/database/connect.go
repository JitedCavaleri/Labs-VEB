package database

import (
	"fmt"
	"log"
	"os"

	"awesomeProject/config"
	"awesomeProject/models"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() *gorm.DB {
	// Явно указываем путь к .env
	if err := godotenv.Load(".env"); err != nil {
		log.Println("⚠️ Не удалось загрузить .env файл:", err)
	} else {
		log.Println("✅ .env файл успешно загружен")
	}

	// Отладка — что именно читается
	log.Println("DB_HOST     =", os.Getenv("DB_HOST"))
	log.Println("DB_USER     =", os.Getenv("DB_USER"))
	log.Println("DB_NAME     =", os.Getenv("DB_NAME"))
	log.Println("DB_PORT     =", os.Getenv("DB_PORT"))
	log.Println("DB_PASSWORD = [скрыто]")

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
	)

	log.Println("Собранный DSN:", dsn)

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Не удалось подключиться к PostgreSQL: ", err)
	}

	log.Println("✅ Подключение к PostgreSQL успешно")

	// Миграции
	err = DB.AutoMigrate(&models.Item{}, &models.User{}, &models.Token{})
	if err != nil {
		log.Fatal("Ошибка миграции: ", err)
	}

	log.Println("✅ Миграции применены успешно")

	config.LoadConfig()

	return DB
}
