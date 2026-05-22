package services

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"awesomeProject/config"
	"awesomeProject/dto"
	"awesomeProject/models"
	"awesomeProject/utils"

	"gorm.io/gorm"
)

// AuthService — бизнес-логика регистрации/входа/обновления токенов.
type AuthService struct {
	DB  *gorm.DB
	Cfg *config.Config
}

func NewAuthService(db *gorm.DB, cfg *config.Config) *AuthService {
	return &AuthService{DB: db, Cfg: cfg}
}

// Ошибки уровня сервиса. Хендлер должен переводить их в HTTP-коды,
// не пропуская техническую информацию.
var (
	ErrEmailAlreadyExists = errors.New("пользователь с таким email уже существует")
	ErrInvalidCredentials = errors.New("неверный email или пароль")
	ErrWeakPassword       = errors.New("пароль должен содержать буквы и цифры")
	ErrInvalidToken       = errors.New("невалидный или истёкший токен")
	ErrUserNotFound       = errors.New("пользователь не найден")
)

// validatePasswordStrength проверяет, что пароль содержит и буквы, и цифры.
// Длина (>=8) проверяется на уровне DTO через binding-теги.
func validatePasswordStrength(password string) error {
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return ErrWeakPassword
	}
	return nil
}

// Register создаёт нового пользователя с хешем пароля и уникальной солью.
func (s *AuthService) Register(req dto.RegisterRequest) (*models.User, error) {
	if err := validatePasswordStrength(req.Password); err != nil {
		return nil, err
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))

	// Проверка уникальности email
	var existing models.User
	if err := s.DB.Where("email = ?", email).First(&existing).Error; err == nil {
		return nil, ErrEmailAlreadyExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	salt, err := utils.GenerateSalt()
	if err != nil {
		return nil, err
	}
	hash, err := utils.HashPassword(req.Password, salt)
	if err != nil {
		return nil, err
	}

	user := models.User{
		Email:        email,
		Phone:        req.Phone,
		PasswordHash: hash,
		Salt:         salt,
	}
	if err := s.DB.Create(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// Login проверяет учётные данные.
func (s *AuthService) Login(req dto.LoginRequest) (*models.User, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	var user models.User
	if err := s.DB.Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Возвращаем одну и ту же ошибку для несуществующего email и для неверного пароля —
			// защита от перебора email'ов (user enumeration).
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if user.PasswordHash == "" {
		// Пользователь зарегистрирован только через OAuth — пароля нет.
		return nil, ErrInvalidCredentials
	}

	if !utils.VerifyPassword(req.Password, user.Salt, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}
	return &user, nil
}

// IssueTokens генерирует пару access+refresh и сохраняет хеш refresh в БД.
// Возвращает строки токенов — их кладёт в cookies хендлер.
func (s *AuthService) IssueTokens(userID uint) (accessToken, refreshToken string, err error) {
	accessToken, _, err = utils.GenerateToken(
		userID, utils.AccessToken,
		s.Cfg.JWTAccessSecret, s.Cfg.JWTAccessExpiration,
	)
	if err != nil {
		return "", "", err
	}

	refreshToken, refreshJTI, err := utils.GenerateToken(
		userID, utils.RefreshToken,
		s.Cfg.JWTRefreshSecret, s.Cfg.JWTRefreshExpiration,
	)
	if err != nil {
		return "", "", err
	}

	// Сохраняем хеш refresh-токена в БД для возможности отзыва.
	tokenRecord := models.Token{
		UserID:    userID,
		TokenHash: utils.HashToken(refreshToken),
		JTI:       refreshJTI,
		ExpiresAt: time.Now().Add(s.Cfg.JWTRefreshExpiration),
		Revoked:   false,
	}
	if err := s.DB.Create(&tokenRecord).Error; err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

// Refresh принимает refresh-токен, валидирует его и выдаёт новую пару.
// Старый refresh при этом отзывается (rotation) — защита от replay-атак.
func (s *AuthService) Refresh(refreshToken string) (newAccess, newRefresh string, err error) {
	claims, err := utils.ParseToken(refreshToken, s.Cfg.JWTRefreshSecret, utils.RefreshToken)
	if err != nil {
		return "", "", ErrInvalidToken
	}

	// Проверяем, что токен есть в БД и не отозван.
	var record models.Token
	if err := s.DB.Where("jti = ? AND token_hash = ?",
		claims.ID, utils.HashToken(refreshToken),
	).First(&record).Error; err != nil {
		return "", "", ErrInvalidToken
	}
	if record.Revoked || record.ExpiresAt.Before(time.Now()) {
		return "", "", ErrInvalidToken
	}

	// Refresh rotation: отзываем старый.
	if err := s.DB.Model(&record).Update("revoked", true).Error; err != nil {
		return "", "", err
	}

	return s.IssueTokens(claims.UserID)
}

// Logout отзывает один конкретный refresh-токен (текущая сессия).
func (s *AuthService) Logout(refreshToken string) error {
	if refreshToken == "" {
		// Нечего отзывать — это не ошибка для пользователя.
		return nil
	}
	hash := utils.HashToken(refreshToken)
	return s.DB.Model(&models.Token{}).
		Where("token_hash = ?", hash).
		Update("revoked", true).Error
}

// LogoutAll отзывает все refresh-токены пользователя (logout со всех устройств).
func (s *AuthService) LogoutAll(userID uint) error {
	return s.DB.Model(&models.Token{}).
		Where("user_id = ? AND revoked = ?", userID, false).
		Update("revoked", true).Error
}

// GetUserByID — получение профиля для /whoami.
func (s *AuthService) GetUserByID(id uint) (*models.User, error) {
	var user models.User
	if err := s.DB.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// ToProfileResponse — безопасная проекция модели в DTO без чувствительных полей.
func ToProfileResponse(u *models.User) dto.UserProfileResponse {
	return dto.UserProfileResponse{
		ID:        u.ID,
		Email:     u.Email,
		Phone:     u.Phone,
		CreatedAt: u.CreatedAt,
	}
}
