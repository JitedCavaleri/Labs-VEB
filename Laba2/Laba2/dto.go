package main

// DTO для создания элемента
type CreateItemDTO struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description" binding:"required"`
}

// DTO для обновления (можно использовать и для PUT, и для PATCH)
type UpdateItemDTO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// DTO для пагинации
type PaginationDTO struct {
	Page  int `form:"page"`
	Limit int `form:"limit"`
}
