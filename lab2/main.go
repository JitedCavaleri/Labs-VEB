package main

import (
	"time"

	"awesomeProject/database"
	"awesomeProject/handlers"
	"awesomeProject/models"
	"awesomeProject/services"

	"github.com/gin-gonic/gin"
)

func daysBeforeNewYear() int {
	now := time.Now()
	nextYear := now.Year() + 1
	newYear := time.Date(nextYear, 1, 1, 0, 0, 0, 0, now.Location())
	return int(newYear.Sub(now).Hours() / 24)
}

func main() {
	db := database.Connect()

	// Миграция
	db.AutoMigrate(&models.Item{})

	// Сервисный слой
	itemService := services.NewItemService(db)

	// Хендлеры
	h := handlers.Handler{Service: itemService}

	r := gin.Default()

	// Старая лаба
	r.GET("/info", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"days_before_new_year": daysBeforeNewYear(),
		})
	})

	// CRUD
	r.POST("/items", h.Create)
	r.GET("/items", h.GetAll)
	r.GET("/items/:id", h.GetByID)
	r.PUT("/items/:id", h.Update)
	r.PATCH("/items/:id", h.Patch) // ← Новый метод по лабе
	r.DELETE("/items/:id", h.Delete)

	r.Run(":4200")
}
