package main

import (
	"time"

	"github.com/google/uuid"
)

// Item = таблица в базе данных
type Item struct {
	ID uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`

	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"Status"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Soft Delete
	DeletedAt *time.Time `json:"deleted_at" gorm:"index"`
}
