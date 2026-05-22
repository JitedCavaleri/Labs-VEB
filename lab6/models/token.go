package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Token — серверная запись о выпущенном refresh-токене (отдельная коллекция).
//
// Почему НЕ embedding в User: refresh-токены ротируются на каждом /auth/refresh
// (token rotation). Вставка в массив user-документа на каждый запрос — это лишняя
// запись 16Kb+ документа целиком. Отдельная коллекция с индексом по userId +
// jti + tokenHash гораздо быстрее и масштабируется лучше.
//
// В БД хранится не сам refresh-токен, а его SHA-256 хеш — это позволяет
// отзывать токены даже при компрометации БД (см. лабу 3).
type Token struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty" swaggertype:"string" example:"6555c0a3d8e94f1b3c5a0002"`
	UserID    primitive.ObjectID `json:"userId" bson:"userId" swaggertype:"string" example:"6555c0a3d8e94f1b3c5a0001"`
	TokenHash string             `json:"-" bson:"tokenHash" swaggerignore:"true"`
	JTI       string             `json:"-" bson:"jti" swaggerignore:"true"`
	ExpiresAt time.Time          `json:"expiresAt" bson:"expiresAt" example:"2025-11-19T10:15:00Z"`
	Revoked   bool               `json:"revoked" bson:"revoked" example:"false"`

	CreatedAt time.Time  `json:"createdAt" bson:"createdAt" example:"2025-11-12T10:15:00Z"`
	UpdatedAt time.Time  `json:"updatedAt" bson:"updatedAt" example:"2025-11-12T10:15:00Z"`
	DeletedAt *time.Time `json:"-" bson:"deletedAt,omitempty" swaggerignore:"true"`
}

// PasswordResetToken — токен сброса пароля (отдельная коллекция).
// Эти токены короткоживущие (1 час) и одноразовые — отдельная коллекция
// упрощает запросы по token_hash и не "пачкает" документ User.
type PasswordResetToken struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	UserID    primitive.ObjectID `json:"userId" bson:"userId"`
	TokenHash string             `json:"-" bson:"tokenHash" swaggerignore:"true"`
	ExpiresAt time.Time          `json:"expiresAt" bson:"expiresAt"`
	Used      bool               `json:"used" bson:"used"`
	CreatedAt time.Time          `json:"createdAt" bson:"createdAt"`
}
