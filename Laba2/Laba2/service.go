package main

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Создание элемента
func CreateItem(input CreateItemDTO) (Item, error) {
	item := Item{
		ID:          uuid.New(),
		Name:        input.Name,
		Description: input.Description,
	}

	if err := DB.Create(&item).Error; err != nil {
		return Item{}, err
	}

	return item, nil
}

// Получение списка с пагинацией
func GetItems(page int, limit int) ([]Item, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	offset := (page - 1) * limit

	var items []Item
	var total int64

	// считаем общее количество
	if err := DB.Model(&Item{}).
		Where("deleted_at IS NULL").
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// получаем данные
	if err := DB.Where("deleted_at IS NULL").
		Limit(limit).
		Offset(offset).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// Получение по ID
func GetItemByID(id string) (Item, error) {
	var item Item

	if err := DB.Where("id = ? AND deleted_at IS NULL", id).
		First(&item).Error; err != nil {
		return Item{}, errors.New("not found")
	}

	return item, nil
}

// Обновление
func UpdateItem(id string, input UpdateItemDTO) (Item, error) {
	var item Item

	if err := DB.Where("id = ? AND deleted_at IS NULL", id).
		First(&item).Error; err != nil {
		return Item{}, errors.New("not found")
	}

	// обновляем только если передано
	if input.Name != "" {
		item.Name = input.Name
	}
	if input.Description != "" {
		item.Description = input.Description
	}

	if err := DB.Save(&item).Error; err != nil {
		return Item{}, err
	}

	return item, nil
}

// Soft Delete
func DeleteItem(id string) error {
	var item Item

	if err := DB.Where("id = ? AND deleted_at IS NULL", id).
		First(&item).Error; err != nil {
		return errors.New("not found")
	}

	now := time.Now()
	item.DeletedAt = &now

	return DB.Save(&item).Error
}
