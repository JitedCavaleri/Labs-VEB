package services

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"strings"
	"time"

	"awesomeProject/dto"
	"awesomeProject/models"
	"awesomeProject/utils"

	"gorm.io/gorm"
)

// PasswordResetService — реализация "forgot password" / "reset password" потока.
type PasswordResetService struct {
	DB *gorm.DB
}

func NewPasswordResetService(db *gorm.DB) *PasswordResetService {
	return &PasswordResetService{DB: db}
}

// CreateResetToken — генерирует reset-токен и сохраняет его хеш в БД.
// В реальном приложении отправляется на email; здесь логируется (для лабы).
// Возвращает токен — в продакшене токен возвращать клиенту НЕЛЬЗЯ, его только высылают на почту.
func (s *PasswordResetService) CreateResetToken(req dto.ForgotPasswordRequest) error {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	var user models.User
	if err := s.DB.Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Не раскрываем существование email — это защита от user enumeration.
			// Возвращаем nil, словно письмо отправили.
			return nil
		}
		return err
	}

	// Генерируем случайный токен.
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	rawToken := base64.URLEncoding.EncodeToString(b)

	record := models.PasswordResetToken{
		UserID:    user.ID,
		TokenHash: utils.HashToken(rawToken),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Used:      false,
	}
	if err := s.DB.Create(&record).Error; err != nil {
		return err
	}

	// В реальной системе — отправить email с этим токеном.
	// В логе показываем ТОЛЬКО первые 8 символов, чтобы не утекал в логи.
	preview := rawToken
	if len(preview) > 8 {
		preview = preview[:8] + "..."
	}
	log.Printf("[password-reset] токен для user_id=%d создан (preview=%s)", user.ID, preview)
	log.Printf("[password-reset] FULL TOKEN (только для отладки лабы): %s", rawToken)

	return nil
}

// ResetPassword — устанавливает новый пароль по reset-токену.
func (s *PasswordResetService) ResetPassword(req dto.ResetPasswordRequest) error {
	hash := utils.HashToken(req.Token)

	var record models.PasswordResetToken
	if err := s.DB.Where("token_hash = ?", hash).First(&record).Error; err != nil {
		return ErrInvalidToken
	}
	if record.Used || record.ExpiresAt.Before(time.Now()) {
		return ErrInvalidToken
	}

	if err := validatePasswordStrength(req.NewPassword); err != nil {
		return err
	}

	var user models.User
	if err := s.DB.First(&user, record.UserID).Error; err != nil {
		return ErrUserNotFound
	}

	// Генерируем новую соль и новый хеш (старые становятся невалидны).
	newSalt, err := utils.GenerateSalt()
	if err != nil {
		return err
	}
	newHash, err := utils.HashPassword(req.NewPassword, newSalt)
	if err != nil {
		return err
	}

	// Транзакция: обновляем пароль, помечаем токен использованным, отзываем все refresh-токены.
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Updates(map[string]interface{}{
			"password_hash": newHash,
			"salt":          newSalt,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&record).Update("used", true).Error; err != nil {
			return err
		}
		// После смены пароля все активные сессии должны быть инвалидированы.
		return tx.Model(&models.Token{}).
			Where("user_id = ? AND revoked = ?", user.ID, false).
			Update("revoked", true).Error
	})
}
