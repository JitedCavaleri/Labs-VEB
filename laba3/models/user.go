package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Email     string         `json:"email" gorm:"uniqueIndex;not null"`
	Password  string         `json:"-"`
	Salt      string         `json:"-"`
	FirstName string         `json:"firstName,omitempty"`
	LastName  string         `json:"lastName,omitempty"`
	YandexID  string         `json:"-" gorm:"index"` // ← только один раз!
	VKID      string         `json:"-" gorm:"index"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}
