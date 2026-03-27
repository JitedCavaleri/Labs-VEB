package dto

import "awesomeProject/models"

// Для создания и полного обновления (POST + PUT)
type ItemCreateRequest struct {
	Name        string `json:"name" binding:"required,min=3,max=100"`
	Description string `json:"description" binding:"required,min=5,max=500"`
}

// Для частичного обновления (PATCH)
type ItemPatchRequest struct {
	Name        string `json:"name" binding:"omitempty,min=3,max=100"`
	Description string `json:"description" binding:"omitempty,min=5,max=500"`
}

// Ответ с пагинацией
type PaginationResponse struct {
	Data []models.Item  `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

type PaginationMeta struct {
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalPages int   `json:"totalPages"`
}
