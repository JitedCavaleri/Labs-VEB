package services

import (
	"awesomeProject/dto"
	"awesomeProject/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserService struct {
	DB *gorm.DB
}

func NewUserService(db *gorm.DB) *UserService {
	return &UserService{DB: db}
}

// Create — создание пользователя с хэшированием пароля
func (s *UserService) Create(req dto.UserCreateRequest) (models.User, error) {
	// Генерируем соль + хеш (bcrypt автоматически использует уникальную соль)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return models.User{}, err
	}

	user := models.User{
		Email:     req.Email,
		Password:  string(hashedPassword), // сохраняем только хеш
		FirstName: req.FirstName,
		LastName:  req.LastName,
		// Salt не храним отдельно — bcrypt уже включает salt в хеш
	}

	err = s.DB.Create(&user).Error
	return user, err
}

// GetAll, GetByID, Update, Patch, Delete — аналогично ItemService
func (s *UserService) GetAll(page, limit int) (dto.PaginationResponse, error) {
	// Реализация пагинации (можно скопировать из ItemService и адаптировать)
	// Пока оставим заглушку — добавим в следующем шаге, если нужно
	return dto.PaginationResponse{}, nil
}

func (s *UserService) GetByID(id uint) (models.User, error) {
	var user models.User
	err := s.DB.First(&user, id).Error
	return user, err
}

func (s *UserService) Update(id uint, req dto.UserUpdateRequest) (models.User, error) {
	var user models.User
	if err := s.DB.First(&user, id).Error; err != nil {
		return models.User{}, err
	}

	user.FirstName = req.FirstName
	user.LastName = req.LastName

	err := s.DB.Save(&user).Error
	return user, err
}

func (s *UserService) Delete(id uint) error {
	return s.DB.Delete(&models.User{}, id).Error
}
