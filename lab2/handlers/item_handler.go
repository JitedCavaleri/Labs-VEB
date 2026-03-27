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

// Create
func (h *Handler) Create(c *gin.Context) {
	var req dto.ItemCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверные данные: " + err.Error()})
		return
	}

	item, err := h.Service.Create(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось создать элемент"})
		return
	}

	c.JSON(http.StatusCreated, item)
}

// GetAll
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

// GetByID
func (h *Handler) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID"})
		return
	}

	item, err := h.Service.GetByID(uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Элемент не найден"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сервера"})
		return
	}

	c.JSON(http.StatusOK, item)
}

// Update (PUT) — полное обновление
func (h *Handler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID"})
		return
	}

	var req dto.ItemCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверные данные: " + err.Error()})
		return
	}

	item, err := h.Service.Update(uint(id), req)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Элемент не найден"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления"})
		return
	}

	c.JSON(http.StatusOK, item)
}

// Patch (PATCH) — частичное обновление
func (h *Handler) Patch(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID"})
		return
	}

	var req dto.ItemPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверные данные: " + err.Error()})
		return
	}

	item, err := h.Service.Patch(uint(id), req)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Элемент не найден"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка частичного обновления"})
		return
	}

	c.JSON(http.StatusOK, item)
}

// Delete
func (h *Handler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID"})
		return
	}

	err = h.Service.Delete(uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Элемент не найден"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления"})
		return
	}

	c.Status(http.StatusNoContent)
}
