package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User — сущность пользователя для MongoDB.
//
// В отличие от лабы 5 (GORM/PostgreSQL):
//   - ID — это ObjectID, а не uint. MongoDB генерирует _id автоматически.
//   - Связи на Token и Item реализованы через ссылки (References) — отдельные коллекции,
//     а не через FK. Это позволяет независимо запрашивать токены/items и не упираться
//     в лимит BSON 16MB при росте данных.
//   - DeletedAt — реализация Soft Delete на уровне приложения. Поле *time.Time:
//     nil == "запись жива", не-nil == "удалена". Все читающие запросы должны
//     явно фильтровать по {"deletedAt": nil} (помощник в database/db.go).
//
// Чувствительные поля помечены `json:"-"` и `swaggerignore:"true"` — они не попадают
// ни в JSON-ответы, ни в схему OpenAPI (требование лабы 4).
type User struct {
	ID    primitive.ObjectID `json:"id" bson:"_id,omitempty" swaggertype:"string" example:"6555c0a3d8e94f1b3c5a0001"`
	Email string             `json:"email" bson:"email" example:"user@example.com"`
	Phone string             `json:"phone,omitempty" bson:"phone,omitempty" example:"+7 999 123-45-67"`

	// хеш пароля (никогда не отдаётся клиенту и не попадает в Swagger)
	PasswordHash string `json:"-" bson:"passwordHash,omitempty" swaggerignore:"true"`
	// уникальная соль для каждого пользователя
	Salt string `json:"-" bson:"salt,omitempty" swaggerignore:"true"`

	// Идентификаторы у OAuth провайдеров (могут быть пустыми)
	YandexID string `json:"-" bson:"yandexId,omitempty" swaggerignore:"true"`
	VkID     string `json:"-" bson:"vkId,omitempty" swaggerignore:"true"`

	CreatedAt time.Time  `json:"createdAt" bson:"createdAt" example:"2025-11-12T10:15:00Z"`
	UpdatedAt time.Time  `json:"updatedAt" bson:"updatedAt" example:"2025-11-12T10:15:00Z"`
	DeletedAt *time.Time `json:"-" bson:"deletedAt,omitempty" swaggerignore:"true"`
}
