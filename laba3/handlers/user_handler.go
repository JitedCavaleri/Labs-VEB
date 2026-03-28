package handlers

import (
	"awesomeProject/dto"
	"awesomeProject/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	Service *services.UserService
}

func (h *UserHandler) Create(c *gin.Context) {
	var req dto.UserCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверные данные: " + err.Error()})
		return
	}

	user, err := h.Service.Create(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось создать пользователя"})
		return
	}

	// Возвращаем безопасный ответ
	response := dto.UserResponse{
		ID:        user.ID,
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
	}

	c.JSON(http.StatusCreated, response)
}

// Остальные методы (GetAll, GetByID, Update, Delete) — можешь добавить позже по аналогии с item_handler.go
// Пока оставляем только Create — он нам нужен для теста хэширования
