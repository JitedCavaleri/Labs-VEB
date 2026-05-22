package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Item — ресурс из Лабораторной №2.
//
// Решение по схеме: отдельная коллекция, ссылка на User через UserID.
// Альтернатива (embedding в User) отвергнута по двум причинам:
//  1. Items может быть произвольно много (пагинация в API!) — упрёмся в 16MB на документ.
//  2. /items/:id ищет ресурс глобально по ID, потом сверяет владельца.
//     Эмбед потребовал бы знать userId до запроса — это ломает текущий API-контракт.
//
// Soft Delete — поле DeletedAt (*time.Time, nil == "жив"). Все читающие запросы
// фильтруют {"deletedAt": nil} (см. helpers в database/db.go).
type Item struct {
	ID          primitive.ObjectID `json:"id" bson:"_id,omitempty" swaggertype:"string" example:"6555c0a3d8e94f1b3c5a0003"`
	Name        string             `json:"name" bson:"name" example:"Молоток"`
	Description string             `json:"description" bson:"description" example:"Стальной молоток с деревянной рукояткой"`
	UserID      primitive.ObjectID `json:"userId" bson:"userId" swaggertype:"string" example:"6555c0a3d8e94f1b3c5a0001"`

	CreatedAt time.Time  `json:"createdAt" bson:"createdAt" example:"2025-11-12T10:15:00Z"`
	UpdatedAt time.Time  `json:"updatedAt" bson:"updatedAt" example:"2025-11-12T10:15:00Z"`
	DeletedAt *time.Time `json:"-" bson:"deletedAt,omitempty" swaggerignore:"true"`
}
