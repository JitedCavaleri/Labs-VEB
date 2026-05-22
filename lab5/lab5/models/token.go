package models

import (
	"time"

	"gorm.io/gorm"
)

// Token — серверная запись о выпущенном refresh-токене.
// В БД хранится не сам токен, а его хеш — это нужно для возможности отзыва
// (logout / logout-all) даже при компрометации БД.
//
// Внимание: эта модель НЕ возвращается через публичные эндпоинты;
// поля TokenHash и JTI помечены `json:"-"` / swaggerignore для подстраховки.
type Token struct {
	ID        uint      `json:"id" gorm:"primaryKey" example:"1"`
	UserID    uint      `json:"userId" gorm:"index;not null" example:"1"`
	User      User      `json:"-" gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" swaggerignore:"true"`
	TokenHash string    `json:"-" gorm:"size:255;index" swaggerignore:"true"` // SHA-256 хеш refresh-токена
	JTI       string    `json:"-" gorm:"size:64;uniqueIndex" swaggerignore:"true"` // уникальный идентификатор JWT
	ExpiresAt time.Time `json:"expiresAt" example:"2025-11-19T10:15:00Z"`
	Revoked   bool      `json:"revoked" gorm:"default:false" example:"false"`

	CreatedAt time.Time      `json:"createdAt" example:"2025-11-12T10:15:00Z"`
	UpdatedAt time.Time      `json:"updatedAt" example:"2025-11-12T10:15:00Z"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index" swaggerignore:"true"`
}

// PasswordResetToken — токен для сброса пароля.
type PasswordResetToken struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"userId" gorm:"index;not null"`
	TokenHash string    `json:"-" gorm:"size:255;index" swaggerignore:"true"`
	ExpiresAt time.Time `json:"expiresAt"`
	Used      bool      `json:"used" gorm:"default:false"`
	CreatedAt time.Time `json:"createdAt"`
}
