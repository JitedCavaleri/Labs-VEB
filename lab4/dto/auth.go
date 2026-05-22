package dto

import "time"

// RegisterRequest — DTO регистрации пользователя.
// Валидируем формат email, длину пароля и наличие достаточной сложности.
type RegisterRequest struct {
	// Email пользователя (уникальный)
	Email string `json:"email" binding:"required,email,max=255" example:"user@example.com"`
	// Телефон в свободном формате (необязательное поле)
	Phone string `json:"phone,omitempty" binding:"omitempty,min=5,max=32" example:"+7 999 123-45-67"`
	// Пароль (минимум 8 символов, обязательно цифры + буквы)
	Password string `json:"password" binding:"required,min=8,max=128" example:"StrongPass123"`
}

// LoginRequest — DTO входа.
type LoginRequest struct {
	// Email пользователя
	Email string `json:"email" binding:"required,email" example:"user@example.com"`
	// Пароль
	Password string `json:"password" binding:"required,min=1,max=128" example:"StrongPass123"`
}

// ForgotPasswordRequest — запрос сброса пароля.
type ForgotPasswordRequest struct {
	// Email пользователя, на который придёт ссылка сброса
	Email string `json:"email" binding:"required,email" example:"user@example.com"`
}

// ResetPasswordRequest — установка нового пароля по reset-токену.
type ResetPasswordRequest struct {
	// Reset-токен, полученный из письма (для лабы — из лога приложения)
	Token string `json:"token" binding:"required,min=10" example:"a1b2c3d4e5f6g7h8i9j0..."`
	// Новый пароль (минимум 8 символов, буквы + цифры)
	NewPassword string `json:"newPassword" binding:"required,min=8,max=128" example:"NewStrongPass456"`
}

// UserProfileResponse — безопасная проекция пользователя для ответа клиенту.
// Не содержит: PasswordHash, Salt, YandexID, VkID, DeletedAt.
type UserProfileResponse struct {
	ID        uint      `json:"id" example:"1"`
	Email     string    `json:"email" example:"user@example.com"`
	Phone     string    `json:"phone,omitempty" example:"+7 999 123-45-67"`
	CreatedAt time.Time `json:"createdAt" example:"2025-11-12T10:15:00Z"`
}

// AuthSuccessResponse — короткий ответ при удачном входе/регистрации.
// Сами токены передаются только в HttpOnly cookies.
type AuthSuccessResponse struct {
	Message string              `json:"message" example:"Успешный вход"`
	User    UserProfileResponse `json:"user"`
}

// MessageResponse — универсальный ответ с одним сообщением.
// Используется для logout, refresh, reset-password и т.д.
type MessageResponse struct {
	Message string `json:"message" example:"Операция выполнена"`
}

// ErrorResponse — единый формат ошибки во всех эндпоинтах.
// В Swagger UI отображается как пример тела для кодов 400/401/403/404/409/500.
type ErrorResponse struct {
	Error string `json:"error" example:"Описание ошибки"`
}

// InfoResponse — ответ публичного эндпоинта /info (из лабы 2).
type InfoResponse struct {
	// Число дней, оставшихся до 1 января следующего года
	DaysBeforeNewYear int `json:"days_before_new_year" example:"42"`
}
