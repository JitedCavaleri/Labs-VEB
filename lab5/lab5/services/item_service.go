package services

import (
	"context"
	"errors"
	"log"

	"awesomeProject/cache"
	"awesomeProject/dto"
	"awesomeProject/models"

	"gorm.io/gorm"
)

// ItemService — CRUD ресурсов с проверкой владения (ownership) и кешированием.
//
// Стратегия кеширования: Cache-Aside (Lazy Loading).
//   - GET сначала идёт в кеш, при miss — в БД, ответ из БД сохраняется в кеш с TTL.
//   - POST/PUT/PATCH/DELETE инвалидируют соответствующие ключи (список + конкретный).
//
// Логика формирования ключей и инвалидации намеренно лежит здесь, в сервисе —
// требование задания: "Логика формирования ключей и инвалидации должна быть реализована
// явно в коде сервиса, а не скрыта в конфигурации".
type ItemService struct {
	DB    *gorm.DB
	Cache *cache.Service
}

// ErrForbidden — пользователь не владелец ресурса.
var ErrForbidden = errors.New("доступ запрещён")

func NewItemService(db *gorm.DB, c *cache.Service) *ItemService {
	return &ItemService{DB: db, Cache: c}
}

// invalidateItemKeys — централизованная инвалидация ключей после любой записи.
// Снимает кеш списков юзера (все страницы — массово по паттерну) и кеш конкретного элемента.
//
// itemID == 0 означает "конкретный элемент неизвестен" (например, при создании) —
// удаляются только списки.
func (s *ItemService) invalidateItemKeys(ctx context.Context, userID, itemID uint) {
	if s.Cache == nil {
		return
	}
	// 1. Все страницы списка этого юзера. Используем DelByPattern (SCAN+UNLINK).
	if err := s.Cache.DelByPattern(ctx, cache.KeyItemsListPatternForUser(userID)); err != nil {
		log.Printf("[items] инвалидация списка user=%d: %v", userID, err)
	}
	// 2. Кеш конкретного элемента.
	if itemID != 0 {
		if err := s.Cache.Del(ctx, cache.KeyItemByID(userID, itemID)); err != nil {
			log.Printf("[items] инвалидация ключа item=%d: %v", itemID, err)
		}
	}
}

// Create — создание элемента, владелец = userID.
// После создания инвалидирует все страницы списка юзера в кеше.
func (s *ItemService) Create(ctx context.Context, userID uint, req dto.ItemCreateRequest) (models.Item, error) {
	item := models.Item{
		Name:        req.Name,
		Description: req.Description,
		UserID:      userID,
	}
	if err := s.DB.Create(&item).Error; err != nil {
		return models.Item{}, err
	}
	// itemID 0: для нового элемента кеша ещё нет, инвалидировать его не нужно.
	s.invalidateItemKeys(ctx, userID, 0)
	return item, nil
}

// GetAll — список с пагинацией, только items текущего пользователя.
// Cache-Aside: сначала смотрим в Redis, при miss — БД + запись в кеш.
func (s *ItemService) GetAll(ctx context.Context, userID uint, page, limit int) (dto.PaginationResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	key := cache.KeyItemsList(userID, page, limit)

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
	offset := (page - 1) * limit
	var items []models.Item
	var total int64

	s.DB.Model(&models.Item{}).Where("user_id = ?", userID).Count(&total)
	s.DB.Where("user_id = ?", userID).Offset(offset).Limit(limit).Find(&items)

	totalPages := (int(total) + limit - 1) / limit

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
func (s *ItemService) GetByID(ctx context.Context, userID, id uint) (models.Item, error) {
	key := cache.KeyItemByID(userID, id)

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
	var item models.Item
	if err := s.DB.First(&item, id).Error; err != nil {
		return models.Item{}, err
	}
	if item.UserID != userID {
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
func (s *ItemService) Update(ctx context.Context, userID, id uint, req dto.ItemCreateRequest) (models.Item, error) {
	item, err := s.getByIDFromDB(userID, id) // важно: НЕ через кеш, чтобы избежать гонок
	if err != nil {
		return models.Item{}, err
	}
	item.Name = req.Name
	item.Description = req.Description
	if err := s.DB.Save(&item).Error; err != nil {
		return models.Item{}, err
	}
	s.invalidateItemKeys(ctx, userID, id)
	return item, nil
}

// Patch — PATCH.
func (s *ItemService) Patch(ctx context.Context, userID, id uint, req dto.ItemPatchRequest) (models.Item, error) {
	item, err := s.getByIDFromDB(userID, id)
	if err != nil {
		return models.Item{}, err
	}

	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if len(updates) == 0 {
		return item, nil
	}
	if err := s.DB.Model(&item).Updates(updates).Error; err != nil {
		return models.Item{}, err
	}
	s.invalidateItemKeys(ctx, userID, id)
	return item, nil
}

// Delete — soft delete, только владельцем + инвалидация кеша.
func (s *ItemService) Delete(ctx context.Context, userID, id uint) error {
	if _, err := s.getByIDFromDB(userID, id); err != nil {
		return err
	}
	if err := s.DB.Delete(&models.Item{}, id).Error; err != nil {
		return err
	}
	s.invalidateItemKeys(ctx, userID, id)
	return nil
}

// getByIDFromDB — внутренняя версия GetByID без кеша.
// Используется операциями записи: они должны видеть актуальное состояние БД,
// иначе можно сохранить устаревшие поля поверх свежих.
func (s *ItemService) getByIDFromDB(userID, id uint) (models.Item, error) {
	var item models.Item
	if err := s.DB.First(&item, id).Error; err != nil {
		return models.Item{}, err
	}
	if item.UserID != userID {
		return models.Item{}, ErrForbidden
	}
	return item, nil
}
