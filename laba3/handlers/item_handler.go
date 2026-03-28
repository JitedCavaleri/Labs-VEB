package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"awesomeProject/dto"
	"awesomeProject/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	Service *services.ItemService
}

func getUserID(c *gin.Context) uint {
	userID, _ := c.Get("user_id")
	return userID.(uint)
}

func (h *Handler) Create(c *gin.Context) {
	var req dto.ItemCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := h.Service.Create(req, getUserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось создать"})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *Handler) GetAll(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	response, err := h.Service.GetAll(page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сервера"})
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) GetByID(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	item, err := h.Service.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Элемент не найден"})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *Handler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID := getUserID(c)

	var req dto.ItemCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := h.Service.Update(uint(id), req, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Элемент не найден или не ваш"})
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "Это не ваша запись"})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *Handler) Patch(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID := getUserID(c)

	var req dto.ItemPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := h.Service.Patch(uint(id), req, userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Это не ваша запись"})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *Handler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID := getUserID(c)

	err := h.Service.Delete(uint(id), userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Это не ваша запись"})
		return
	}
	c.Status(http.StatusNoContent)
}
