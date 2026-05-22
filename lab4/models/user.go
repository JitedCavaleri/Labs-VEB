package models

import (
	"time"

	"gorm.io/gorm"
)

// User — сущность пользователя.
// Хранит email, телефон, хеш пароля и соль, идентификаторы OAuth провайдеров.
//
// Чувствительные поля (PasswordHash, Salt, YandexID, VkID, DeletedAt)
// помечены `json:"-"` и `swaggerignore:"true"` — они не появляются ни в JSON-ответах,
// ни в схеме OpenAPI. См. лабу 4, требование "Безопасность в UI".
type User struct {
	ID    uint   `json:"id" gorm:"primaryKey" example:"1"`
	Email string `json:"email" gorm:"uniqueIndex;size:255" example:"user@example.com"`
	Phone string `json:"phone,omitempty" gorm:"size:32" example:"+7 999 123-45-67"`

	// хеш пароля (никогда не отдаётся клиенту и не попадает в Swagger)
	PasswordHash string `json:"-" gorm:"size:255" swaggerignore:"true"`
	// уникальная соль для каждого пользователя
	Salt string `json:"-" gorm:"size:255" swaggerignore:"true"`

	// Идентификаторы у OAuth провайдеров (могут быть пустыми)
	YandexID string `json:"-" gorm:"size:128;index" swaggerignore:"true"`
	VkID     string `json:"-" gorm:"size:128;index" swaggerignore:"true"`

	CreatedAt time.Time      `json:"createdAt" example:"2025-11-12T10:15:00Z"`
	UpdatedAt time.Time      `json:"updatedAt" example:"2025-11-12T10:15:00Z"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index" swaggerignore:"true"`
}
