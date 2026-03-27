package main

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// POST /items
func createItemHandler(c *gin.Context) {
	var input CreateItemDTO

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := CreateItem(input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, item)
}

// GET /items
func getItemsHandler(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	items, total, err := GetItems(page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": items,
		"meta": gin.H{
			"total": total,
			"page":  page,
			"limit": limit,
		},
	})
}

// GET /items/:id
func getItemByIDHandler(c *gin.Context) {
	id := c.Param("id")

	item, err := GetItemByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	c.JSON(http.StatusOK, item)
}

// PUT/PATCH /items/:id
func updateItemHandler(c *gin.Context) {
	id := c.Param("id")

	var input UpdateItemDTO

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := UpdateItem(id, input)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	c.JSON(http.StatusOK, item)
}

// DELETE /items/:id
func deleteItemHandler(c *gin.Context) {
	id := c.Param("id")

	if err := DeleteItem(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	c.Status(http.StatusNoContent)
}
