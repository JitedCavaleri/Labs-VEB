package services

import (
	"awesomeProject/dto"
	"awesomeProject/models"

	"gorm.io/gorm"
)

type ItemService struct {
	DB *gorm.DB
}

func NewItemService(db *gorm.DB) *ItemService {
	return &ItemService{DB: db}
}

// Create — создание элемента
func (s *ItemService) Create(req dto.ItemCreateRequest) (models.Item, error) {
	item := models.Item{
		Name:        req.Name,
		Description: req.Description,
	}
	err := s.DB.Create(&item).Error
	return item, err
}

// GetAll — список с пагинацией
func (s *ItemService) GetAll(page, limit int) (dto.PaginationResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	offset := (page - 1) * limit

	var items []models.Item
	var total int64

	s.DB.Model(&models.Item{}).Count(&total)
	s.DB.Offset(offset).Limit(limit).Find(&items)

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

// GetByID
func (s *ItemService) GetByID(id uint) (models.Item, error) {
	var item models.Item
	err := s.DB.First(&item, id).Error
	return item, err
}

// Update — полное обновление (PUT)
func (s *ItemService) Update(id uint, req dto.ItemCreateRequest) (models.Item, error) {
	var item models.Item
	if err := s.DB.First(&item, id).Error; err != nil {
		return models.Item{}, err
	}

	item.Name = req.Name
	item.Description = req.Description

	err := s.DB.Save(&item).Error
	return item, err
}

// Patch — частичное обновление (PATCH)
func (s *ItemService) Patch(id uint, req dto.ItemPatchRequest) (models.Item, error) {
	var item models.Item
	if err := s.DB.First(&item, id).Error; err != nil {
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

	err := s.DB.Model(&item).Updates(updates).Error
	return item, err
}

// Delete — мягкое удаление
func (s *ItemService) Delete(id uint) error {
	return s.DB.Delete(&models.Item{}, id).Error
}
