// Package database — слой подключения к MongoDB.
//
// Лаба 6: вместо PostgreSQL/GORM используется MongoDB через официальный
// драйвер go.mongodb.org/mongo-driver. Подход:
//
//   - URI берётся из переменной окружения MONGO_URI (см. .env).
//   - Имя БД — из URI (если в URI есть /<db>) или из DB_NAME.
//   - При старте создаются необходимые индексы (unique email, индексы по userId, tokenHash и т.д.).
//   - Soft Delete реализован на уровне приложения: поле deletedAt (nil == "жив").
//     Хелпер AliveFilter() даёт {"deletedAt": nil} для подмешивания во все читающие запросы.
//
// "Миграция" в MongoDB — это создание индексов (схема гибкая, таблицы не нужны).
package database

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Имена коллекций централизованы в одном месте.
// Использование в коде через эти константы убирает риск опечаток.
const (
	CollUsers               = "users"
	CollTokens              = "tokens"
	CollPasswordResetTokens = "password_reset_tokens"
	CollItems               = "items"
)

// DB — обёртка над *mongo.Client + *mongo.Database, чтобы сервисам было удобно
// получать конкретные коллекции и не таскать клиент.
type DB struct {
	Client *mongo.Client
	Mongo  *mongo.Database
}

// Connect устанавливает соединение с MongoDB с retry-стратегией (важно при старте в docker-compose,
// когда mongo-контейнер ещё не успел подняться).
//
// URI читается из MONGO_URI; имя БД — либо из path-части URI, либо из DB_NAME (fallback).
func Connect() *DB {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		log.Fatal("MONGO_URI не задан в окружении")
	}
	dbName := dbNameFromURI(uri)
	if dbName == "" {
		dbName = os.Getenv("DB_NAME")
	}
	if dbName == "" {
		log.Fatal("Не удалось определить имя БД (нет ни /<db> в MONGO_URI, ни DB_NAME)")
	}

	opts := options.Client().
		ApplyURI(uri).
		SetConnectTimeout(5 * time.Second).
		SetServerSelectionTimeout(5 * time.Second)

	var client *mongo.Client
	var err error

	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		client, err = mongo.Connect(ctx, opts)
		if err == nil {
			if pingErr := client.Ping(ctx, nil); pingErr == nil {
				cancel()
				log.Printf("Успешное подключение к MongoDB (db=%s)", dbName)
				return &DB{Client: client, Mongo: client.Database(dbName)}
			} else {
				err = pingErr
			}
		}
		cancel()
		log.Printf("Попытка %d/30: не удалось подключиться к MongoDB: %v. Жду 2 секунды...", i+1, err)
		time.Sleep(2 * time.Second)
	}

	log.Fatal("Не удалось подключиться к MongoDB после 30 попыток")
	return nil
}

// dbNameFromURI вытаскивает имя БД из URI вида mongodb://user:pass@host:port/<dbName>?params.
// Возвращает пустую строку, если path-части в URI нет.
func dbNameFromURI(uri string) string {
	// Срезаем схему.
	idx := strings.Index(uri, "://")
	if idx < 0 {
		return ""
	}
	rest := uri[idx+3:]
	// Срезаем query (?...).
	if q := strings.Index(rest, "?"); q >= 0 {
		rest = rest[:q]
	}
	// Path — после первого '/'
	slash := strings.Index(rest, "/")
	if slash < 0 || slash == len(rest)-1 {
		return ""
	}
	return rest[slash+1:]
}

// Migrate в MongoDB — это создание индексов.
// В отличие от SQL, схемы как таковой нет, и DDL-миграций тоже.
// Но без индексов: уникальность email не гарантируется, выборки по userId медленные,
// а валидные access-JTI ищутся полным сканом. Поэтому индексы — обязательны.
func Migrate(db *DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// users.email — UNIQUE (защищает от двух пользователей с одинаковым email).
	// partialFilterExpression: только для "живых" документов (deletedAt == null), чтобы
	// после soft-delete можно было повторно зарегистрировать тот же email при необходимости.
	if _, err := db.Mongo.Collection(CollUsers).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "email", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetName("uniq_email_alive").
				SetPartialFilterExpression(bson.M{"deletedAt": bson.M{"$eq": nil}}),
		},
		{Keys: bson.D{{Key: "yandexId", Value: 1}}, Options: options.Index().SetSparse(true).SetName("idx_yandexId")},
		{Keys: bson.D{{Key: "vkId", Value: 1}}, Options: options.Index().SetSparse(true).SetName("idx_vkId")},
		{Keys: bson.D{{Key: "deletedAt", Value: 1}}, Options: options.Index().SetName("idx_deletedAt")},
	}); err != nil {
		return fmt.Errorf("индексы users: %w", err)
	}

	// tokens — для быстрого поиска при /auth/refresh, /auth/logout, /auth/logout-all.
	if _, err := db.Mongo.Collection(CollTokens).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "userId", Value: 1}}, Options: options.Index().SetName("idx_userId")},
		{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: options.Index().SetName("idx_tokenHash")},
		{
			Keys:    bson.D{{Key: "jti", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_jti"),
		},
		{Keys: bson.D{{Key: "deletedAt", Value: 1}}, Options: options.Index().SetName("idx_deletedAt")},
	}); err != nil {
		return fmt.Errorf("индексы tokens: %w", err)
	}

	// password_reset_tokens — поиск идёт по token_hash.
	if _, err := db.Mongo.Collection(CollPasswordResetTokens).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: options.Index().SetName("idx_tokenHash")},
		{Keys: bson.D{{Key: "userId", Value: 1}}, Options: options.Index().SetName("idx_userId")},
	}); err != nil {
		return fmt.Errorf("индексы password_reset_tokens: %w", err)
	}

	// items — частая выборка списка ресурсов конкретного пользователя.
	if _, err := db.Mongo.Collection(CollItems).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "userId", Value: 1}}, Options: options.Index().SetName("idx_userId")},
		{Keys: bson.D{{Key: "deletedAt", Value: 1}}, Options: options.Index().SetName("idx_deletedAt")},
	}); err != nil {
		return fmt.Errorf("индексы items: %w", err)
	}

	log.Println("Индексы MongoDB созданы успешно")
	return nil
}

// Disconnect аккуратно закрывает соединение (вызывать через defer в main.go).
func (d *DB) Disconnect() {
	if d == nil || d.Client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Client.Disconnect(ctx); err != nil {
		log.Printf("Ошибка закрытия MongoDB-соединения: %v", err)
	}
}

// --- Helpers для построения фильтров ---

// AliveFilter — фильтр "запись не удалена" (soft delete).
// Его нужно подмешивать в ЛЮБОЙ читающий запрос (Find, FindOne, CountDocuments).
//
// MongoDB при сравнении с null матчит как документы где поле == null,
// так и документы где поля просто нет, поэтому индекс по deletedAt полезен.
func AliveFilter() bson.M {
	return bson.M{"deletedAt": nil}
}

// MergeAlive объединяет произвольный пользовательский фильтр с условием "не удалён".
// Если в filter уже есть ключ deletedAt — не перезаписываем (на случай, если запрос
// сознательно ищет удалённые).
func MergeAlive(filter bson.M) bson.M {
	if filter == nil {
		return AliveFilter()
	}
	if _, ok := filter["deletedAt"]; !ok {
		filter["deletedAt"] = nil
	}
	return filter
}

// ErrNotFound — типизированная ошибка "нет такого документа", независимая от драйвера.
// Сервисы возвращают её хендлерам, которые мапят в HTTP 404.
var ErrNotFound = errors.New("document not found")

// IsNotFound — обёртка над mongo.ErrNoDocuments + наш ErrNotFound.
func IsNotFound(err error) bool {
	return errors.Is(err, mongo.ErrNoDocuments) || errors.Is(err, ErrNotFound)
}
