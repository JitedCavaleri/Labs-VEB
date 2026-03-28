package main

import (
	"time"

	"awesomeProject/database"
	"awesomeProject/handlers"
	"awesomeProject/middleware"
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

	// Сервисы
	itemService := services.NewItemService(db)
	userService := services.NewUserService(db)
	authService := services.NewAuthService(db, userService)

	// Хендлеры
	itemHandler := handlers.Handler{Service: itemService}
	authHandler := handlers.AuthHandler{AuthService: authService}

	r := gin.Default()

	// Старая лаба (инфо)
	r.GET("/info", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"days_before_new_year": daysBeforeNewYear(),
		})
	})

	// ====================== CRUD Items (защищённые) ======================
	r.POST("/items", middleware.AuthMiddleware(), itemHandler.Create)
	r.GET("/items", itemHandler.GetAll) // список можно оставить публичным
	r.GET("/items/:id", itemHandler.GetByID)
	r.PUT("/items/:id", middleware.AuthMiddleware(), itemHandler.Update)
	r.PATCH("/items/:id", middleware.AuthMiddleware(), itemHandler.Patch)
	r.DELETE("/items/:id", middleware.AuthMiddleware(), itemHandler.Delete)

	// ====================== Auth (ЛР3) ======================
	r.POST("/auth/register", authHandler.Register)
	r.POST("/auth/login", authHandler.Login)
	r.POST("/auth/refresh", authHandler.Refresh)
	r.POST("/auth/logout", authHandler.Logout)
	r.POST("/auth/logout-all", middleware.AuthMiddleware(), authHandler.LogoutAll)
	r.GET("/auth/whoami", middleware.AuthMiddleware(), authHandler.Whoami)

	// ====================== OAuth Yandex ======================
	r.GET("/auth/oauth/yandex", authHandler.OAuthYandex)
	r.GET("/auth/oauth/yandex/callback", authHandler.OAuthYandexCallback)

	r.Run(":4200")
}
