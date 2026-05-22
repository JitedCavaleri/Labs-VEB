package handlers

import (
	"net/http"
	"time"

	"awesomeProject/dto"

	"github.com/gin-gonic/gin"
)

// daysBeforeNewYear — сколько дней осталось до 1 января следующего года.
func daysBeforeNewYear() int {
	now := time.Now()
	nextYear := now.Year() + 1
	newYear := time.Date(nextYear, 1, 1, 0, 0, 0, 0, now.Location())
	return int(newYear.Sub(now).Hours() / 24)
}

// Info godoc
// @Summary      Публичный info-эндпоинт
// @Description  Возвращает число дней до Нового года (из Лабораторной №2). Не требует авторизации.
// @Tags         Public
// @Produce      json
// @Success      200  {object}  dto.InfoResponse
// @Router       /info [get]
func Info(c *gin.Context) {
	c.JSON(http.StatusOK, dto.InfoResponse{
		DaysBeforeNewYear: daysBeforeNewYear(),
	})
}
