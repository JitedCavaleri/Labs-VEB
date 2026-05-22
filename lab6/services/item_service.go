package services

import (
	"context"
	"errors"
	"log"
	"time"

	"awesomeProject/cache"
	"awesomeProject/database"
	"awesomeProject/dto"
	"awesomeProject/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ItemService — CRUD ресурсов с проверкой владения (ownership) и кешированием.
//
// Лаба 6: работа с MongoDB через драйвер. Soft Delete — это field-set deletedAt,
// а не DELETE-запрос. Все читающие запросы фильтруются {"deletedAt": nil}.
//
// Стратегия кеширования (Cache-Aside, Lazy Loading) сохранена из лабы 5:
//   - GET сначала идёт в кеш, при miss — в БД, ответ из БД сохраняется в кеш с TTL.
//   - POST/PUT/PATCH/DELETE инвалидируют соответствующие ключи (список + конкретный).
type ItemService struct {
	DB    *database.DB
	Cache *cache.Service
}

// ErrForbidden — пользователь не владелец ресурса.
var ErrForbidden = errors.New("доступ запрещён")

func NewItemService(db *database.DB, c *cache.Service) *ItemService {
	return &ItemService{DB: db, Cache: c}
}

func (s *ItemService) coll() *mongo.Collection { return s.DB.Mongo.Collection(database.CollItems) }

// invalidateItemKeys — централизованная инвалидация ключей после любой записи.
// Снимает кеш списков юзера (все страницы — массово по паттерну) и кеш конкретного элемента.
//
// itemID == "" означает "конкретный элемент неизвестен" (например, при создании) —
// удаляются только списки.
func (s *ItemService) invalidateItemKeys(ctx context.Context, userIDHex, itemIDHex string) {
	if s.Cache == nil {
		return
	}
	// 1. Все страницы списка этого юзера. Используем DelByPattern (SCAN+UNLINK).
	if err := s.Cache.DelByPattern(ctx, cache.KeyItemsListPatternForUser(userIDHex)); err != nil {
		log.Printf("[items] инвалидация списка user=%s: %v", userIDHex, err)
	}
	// 2. Кеш конкретного элемента.
	if itemIDHex != "" {
		if err := s.Cache.Del(ctx, cache.KeyItemByID(userIDHex, itemIDHex)); err != nil {
			log.Printf("[items] инвалидация ключа item=%s: %v", itemIDHex, err)
		}
	}
}

// Create — создание элемента, владелец = userID.
// После создания инвалидирует все страницы списка юзера в кеше.
func (s *ItemService) Create(ctx context.Context, userIDHex string, req dto.ItemCreateRequest) (models.Item, error) {
	userID, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		return models.Item{}, ErrForbidden
	}

	now := time.Now().UTC()
	item := models.Item{
		ID:          primitive.NewObjectID(),
		Name:        req.Name,
		Description: req.Description,
		UserID:      userID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	dbctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := s.coll().InsertOne(dbctx, item); err != nil {
		return models.Item{}, err
	}
	// itemID "" : для нового элемента кеша ещё нет, инвалидировать его не нужно.
	s.invalidateItemKeys(ctx, userIDHex, "")
	return item, nil
}

// GetAll — список с пагинацией, только items текущего пользователя.
// Cache-Aside: сначала смотрим в Redis, при miss — БД + запись в кеш.
//
// Пагинация в MongoDB реализована через skip + limit:
//   - skip = (page - 1) * limit
//   - cursor нам не подходит, т.к. UI ожидает классические страницы.
//
// Сортировка по _id desc нужна, чтобы skip/limit давали стабильный порядок
// (без явной сортировки Mongo не гарантирует порядок документов в курсоре).
func (s *ItemService) GetAll(ctx context.Context, userIDHex string, page, limit int) (dto.PaginationResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	userID, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		return dto.PaginationResponse{}, ErrForbidden
	}

	key := cache.KeyItemsList(userIDHex, page, limit)

	// 1. Пробуем кеш.
	var cached dto.PaginationResponse
	if s.Cache != nil {
		if err := s.Cache.Get(ctx, key, &cached); err == nil {
			log.Printf("[items] HIT  %s", key)
			return cached, nil
		} else if !errors.Is(err, cache.ErrCacheMiss) && !errors.Is(err, cache.ErrCacheUnavailable) {
			log.Printf("[items] кеш-ошибка %s: %v (фолбэк в БД)", key, err)
		}
	}

	// 2. Cache miss — идём в БД.
	log.Printf("[items] MISS %s — запрос в БД", key)

	dbctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	filter := database.MergeAlive(bson.M{"userId": userID})

	total, err := s.coll().CountDocuments(dbctx, filter)
	if err != nil {
		return dto.PaginationResponse{}, err
	}

	skip := int64((page - 1) * limit)
	findOpts := options.Find().
		SetSkip(skip).
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "_id", Value: -1}}) // стабильный порядок (новые сверху)

	cur, err := s.coll().Find(dbctx, filter, findOpts)
	if err != nil {
		return dto.PaginationResponse{}, err
	}
	defer cur.Close(dbctx)

	items := []models.Item{}
	if err := cur.All(dbctx, &items); err != nil {
		return dto.PaginationResponse{}, err
	}

	totalPages := 0
	if limit > 0 {
		totalPages = (int(total) + limit - 1) / limit
	}

	resp := dto.PaginationResponse{
		Data: items,
		Meta: dto.PaginationMeta{
			Total:      total,
			Page:       page,
			Limit:      limit,
			TotalPages: totalPages,
		},
	}

	// 3. Записываем в кеш. TTL=0 → возьмётся defaultTTL из конфига.
	if s.Cache != nil {
		_ = s.Cache.Set(ctx, key, resp, 0)
	}

	return resp, nil
}

// GetByID — доступ только владельцу. Cache-Aside для конкретного элемента.
func (s *ItemService) GetByID(ctx context.Context, userIDHex, idHex string) (models.Item, error) {
	id, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return models.Item{}, database.ErrNotFound
	}

	key := cache.KeyItemByID(userIDHex, idHex)

	// 1. Кеш.
	if s.Cache != nil {
		var cached models.Item
		if err := s.Cache.Get(ctx, key, &cached); err == nil {
			log.Printf("[items] HIT  %s", key)
			return cached, nil
		}
	}

	// 2. БД.
	log.Printf("[items] MISS %s — запрос в БД", key)
	dbctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var item models.Item
	err = s.coll().FindOne(dbctx, database.MergeAlive(bson.M{"_id": id})).Decode(&item)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return models.Item{}, database.ErrNotFound
		}
		return models.Item{}, err
	}
	if item.UserID.Hex() != userIDHex {
		// Чужой ресурс — в кеш НЕ сохраняем (иначе помешаем настоящему владельцу).
		return models.Item{}, ErrForbidden
	}

	// 3. Запись в кеш.
	if s.Cache != nil {
		_ = s.Cache.Set(ctx, key, item, 0)
	}
	return item, nil
}

// Update — PUT. Инвалидирует кеш списка и кеш конкретного элемента.
func (s *ItemService) Update(ctx context.Context, userIDHex, idHex string, req dto.ItemCreateRequest) (models.Item, error) {
	item, err := s.getByIDFromDB(ctx, userIDHex, idHex) // важно: НЕ через кеш, чтобы избежать гонок
	if err != nil {
		return models.Item{}, err
	}

	now := time.Now().UTC()
	dbctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	upd := bson.M{"$set": bson.M{
		"name":        req.Name,
		"description": req.Description,
		"updatedAt":   now,
	}}
	if _, err := s.coll().UpdateByID(dbctx, item.ID, upd); err != nil {
		return models.Item{}, err
	}
	item.Name = req.Name
	item.Description = req.Description
	item.UpdatedAt = now

	s.invalidateItemKeys(ctx, userIDHex, idHex)
	return item, nil
}

// Patch — PATCH. Только переданные поля.
func (s *ItemService) Patch(ctx context.Context, userIDHex, idHex string, req dto.ItemPatchRequest) (models.Item, error) {
	item, err := s.getByIDFromDB(ctx, userIDHex, idHex)
	if err != nil {
		return models.Item{}, err
	}

	updates := bson.M{}
	if req.Name != "" {
		updates["name"] = req.Name
		item.Name = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
		item.Description = req.Description
	}
	if len(updates) == 0 {
		return item, nil
	}
	now := time.Now().UTC()
	updates["updatedAt"] = now
	item.UpdatedAt = now

	dbctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := s.coll().UpdateByID(dbctx, item.ID, bson.M{"$set": updates}); err != nil {
		return models.Item{}, err
	}

	s.invalidateItemKeys(ctx, userIDHex, idHex)
	return item, nil
}

// Delete — soft delete, только владельцем + инвалидация кеша.
// Soft Delete = установка deletedAt в текущий момент времени; документ физически остаётся.
func (s *ItemService) Delete(ctx context.Context, userIDHex, idHex string) error {
	if _, err := s.getByIDFromDB(ctx, userIDHex, idHex); err != nil {
		return err
	}
	id, _ := primitive.ObjectIDFromHex(idHex) // выше уже проверили валидность

	now := time.Now().UTC()
	upd := bson.M{"$set": bson.M{
		"deletedAt": now,
		"updatedAt": now,
	}}

	dbctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := s.coll().UpdateByID(dbctx, id, upd); err != nil {
		return err
	}
	s.invalidateItemKeys(ctx, userIDHex, idHex)
	return nil
}

// getByIDFromDB — внутренняя версия GetByID без кеша.
// Используется операциями записи: они должны видеть актуальное состояние БД,
// иначе можно сохранить устаревшие поля поверх свежих.
func (s *ItemService) getByIDFromDB(ctx context.Context, userIDHex, idHex string) (models.Item, error) {
	id, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return models.Item{}, database.ErrNotFound
	}

	dbctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var item models.Item
	err = s.coll().FindOne(dbctx, database.MergeAlive(bson.M{"_id": id})).Decode(&item)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return models.Item{}, database.ErrNotFound
		}
		return models.Item{}, err
	}
	if item.UserID.Hex() != userIDHex {
		return models.Item{}, ErrForbidden
	}
	return item, nil
}
