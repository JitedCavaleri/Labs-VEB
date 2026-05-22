package dto

import "awesomeProject/models"

// ItemCreateRequest — создание / полное обновление (POST/PUT).
type ItemCreateRequest struct {
	// Название ресурса (3..100 символов)
	Name string `json:"name" binding:"required,min=3,max=100" example:"Молоток"`
	// Описание ресурса (5..500 символов)
	Description string `json:"description" binding:"required,min=5,max=500" example:"Стальной молоток с деревянной рукояткой"`
}

// ItemPatchRequest — частичное обновление (PATCH).
// Все поля опциональны; передаются только те, которые требуется обновить.
type ItemPatchRequest struct {
	// Новое название (опционально)
	Name string `json:"name,omitempty" binding:"omitempty,min=3,max=100" example:"Новый молоток"`
	// Новое описание (опционально)
	Description string `json:"description,omitempty" binding:"omitempty,min=5,max=500" example:"Обновлённое описание"`
}

// PaginationResponse — ответ списка с пагинацией.
// Лаба 6: в MongoDB пагинация реализована через skip+limit (см. ItemService.GetAll).
type PaginationResponse struct {
	Data []models.Item  `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

// PaginationMeta — метаданные пагинации.
type PaginationMeta struct {
	Total      int64 `json:"total" example:"42"`
	Page       int   `json:"page" example:"1"`
	Limit      int   `json:"limit" example:"10"`
	TotalPages int   `json:"totalPages" example:"5"`
}
