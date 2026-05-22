package services

import (
	"errors"

	"awesomeProject/dto"
	"awesomeProject/models"

	"gorm.io/gorm"
)

// ItemService — CRUD ресурсов с проверкой владения (ownership).
// Все операции изменения / получения по ID должны проверять, что item принадлежит userID.
type ItemService struct {
	DB *gorm.DB
}

// ErrForbidden — пользователь не владелец ресурса.
var ErrForbidden = errors.New("доступ запрещён")

func NewItemService(db *gorm.DB) *ItemService {
	return &ItemService{DB: db}
}

// Create — создание элемента, владелец = userID.
func (s *ItemService) Create(userID uint, req dto.ItemCreateRequest) (models.Item, error) {
	item := models.Item{
		Name:        req.Name,
		Description: req.Description,
		UserID:      userID,
	}
	err := s.DB.Create(&item).Error
	return item, err
}

// GetAll — список с пагинацией, только items текущего пользователя.
func (s *ItemService) GetAll(userID uint, page, limit int) (dto.PaginationResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit

	var items []models.Item
	var total int64

	s.DB.Model(&models.Item{}).Where("user_id = ?", userID).Count(&total)
	s.DB.Where("user_id = ?", userID).Offset(offset).Limit(limit).Find(&items)

	totalPages := (int(total) + limit - 1) / limit

	return dto.PaginationResponse{
		Data: items,
		Meta: dto.PaginationMeta{
			Total:      total,
			Page:       page,
			Limit:      limit,
			TotalPages: totalPages,
		},
	}, nil
}

// GetByID — доступ только владельцу.
// Возвращает ErrForbidden, если item существует, но принадлежит другому пользователю.
func (s *ItemService) GetByID(userID, id uint) (models.Item, error) {
	var item models.Item
	if err := s.DB.First(&item, id).Error; err != nil {
		return models.Item{}, err
	}
	if item.UserID != userID {
		return models.Item{}, ErrForbidden
	}
	return item, nil
}

// Update — PUT.
func (s *ItemService) Update(userID, id uint, req dto.ItemCreateRequest) (models.Item, error) {
	item, err := s.GetByID(userID, id)
	if err != nil {
		return models.Item{}, err
	}
	item.Name = req.Name
	item.Description = req.Description
	err = s.DB.Save(&item).Error
	return item, err
}

// Patch — PATCH.
func (s *ItemService) Patch(userID, id uint, req dto.ItemPatchRequest) (models.Item, error) {
	item, err := s.GetByID(userID, id)
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
	err = s.DB.Model(&item).Updates(updates).Error
	return item, err
}

// Delete — soft delete, только владельцем.
func (s *ItemService) Delete(userID, id uint) error {
	_, err := s.GetByID(userID, id)
	if err != nil {
		return err
	}
	return s.DB.Delete(&models.Item{}, id).Error
}
