package models

import (
	"time"

	"gorm.io/gorm"
)

// Item — ресурс из Лабораторной №2.
// Добавлено поле UserID для реализации проверки владения (только владелец может редактировать/удалять).
type Item struct {
	ID          uint           `json:"id" gorm:"primaryKey" example:"1"`
	Name        string         `json:"name" example:"Молоток"`
	Description string         `json:"description" example:"Стальной молоток с деревянной рукояткой"`
	UserID      uint           `json:"userId" gorm:"index;not null" example:"1"`
	User        User           `json:"-" gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" swaggerignore:"true"`
	CreatedAt   time.Time      `json:"createdAt" example:"2025-11-12T10:15:00Z"`
	UpdatedAt   time.Time      `json:"updatedAt" example:"2025-11-12T10:15:00Z"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index" swaggerignore:"true"`
}
