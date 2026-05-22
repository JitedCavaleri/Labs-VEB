package services

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"
	"unicode"

	"awesomeProject/cache"
	"awesomeProject/config"
	"awesomeProject/dto"
	"awesomeProject/models"
	"awesomeProject/utils"

	"gorm.io/gorm"
)

// AuthService — бизнес-логика регистрации/входа/обновления токенов.
//
// Лаба 5: добавлены два механизма поверх лабы 3/4:
//  1. Реестр валидных access-токенов в Redis (по JTI). Позволяет мгновенно отозвать
//     access токен до его естественного истечения (см. /auth/logout).
//  2. Кеш профиля пользователя для /auth/whoami (cache-aside + инвалидация при logout).
type AuthService struct {
	DB    *gorm.DB
	Cfg   *config.Config
	Cache *cache.Service
}

func NewAuthService(db *gorm.DB, cfg *config.Config, c *cache.Service) *AuthService {
	return &AuthService{DB: db, Cfg: cfg, Cache: c}
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

// IssueTokens генерирует пару access+refresh, сохраняет хеш refresh в БД
// и регистрирует JTI access-токена в Redis с TTL = время жизни access.
//
// JTI в Redis нужен, чтобы реализовать мгновенный Logout. JWT по своей природе stateless:
// без внешней проверки токен живёт до expiry. Запись JTI в Redis превращает access
// в "stateful" — middleware на каждом запросе сверяется с реестром.
//
// В кеш кладётся ТОЛЬКО идентификатор (JTI), а не сам токен — это требование безопасности
// из задания: "Запрещено хранение полных токенов доступа в открытом виде".
func (s *AuthService) IssueTokens(userID uint) (accessToken, refreshToken string, err error) {
	accessToken, accessJTI, err := utils.GenerateToken(
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

	// 1. Refresh — в БД (как в лабе 3). Долгоживущий, точная отзываемость через таблицу.
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

	// 2. Access JTI — в Redis с TTL = время жизни access токена.
	// Когда TTL истечёт — ключ исчезнет автоматически, дополнительная очистка не нужна.
	// Значение — "valid" (можно было бы хранить userID, но в ключе он уже есть).
	if s.Cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.Cache.SetString(
			ctx,
			cache.KeyAccessJTI(userID, accessJTI),
			"valid",
			s.Cfg.JWTAccessExpiration,
		); err != nil {
			// Не падаем: если Redis недоступен, middleware у нас сконфигурирован
			// в "fail-open" режиме (см. middleware/auth.go).
			log.Printf("[auth] не удалось сохранить JTI в Redis: %v", err)
		}
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

// Logout отзывает один конкретный refresh-токен (текущая сессия)
// + удаляет JTI access токена из Redis (мгновенный отзыв доступа).
// + сбрасывает кеш профиля.
//
// accessJTI приходит из middleware (он положил JTI в gin.Context на запросе).
func (s *AuthService) Logout(refreshToken string, userID uint, accessJTI string) error {
	// 1. Отзыв refresh в БД.
	if refreshToken != "" {
		hash := utils.HashToken(refreshToken)
		if err := s.DB.Model(&models.Token{}).
			Where("token_hash = ?", hash).
			Update("revoked", true).Error; err != nil {
			return err
		}
	}

	// 2. Отзыв access JTI в Redis и инвалидация кеша профиля.
	if s.Cache != nil && userID != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		keys := []string{cache.KeyUserProfile(userID)}
		if accessJTI != "" {
			keys = append(keys, cache.KeyAccessJTI(userID, accessJTI))
		}
		if err := s.Cache.Del(ctx, keys...); err != nil {
			log.Printf("[auth] Logout: ошибка удаления ключей из Redis: %v", err)
		}
	}
	return nil
}

// LogoutAll отзывает все refresh-токены пользователя (logout со всех устройств)
// + удаляет ВСЕ JTI access-токенов пользователя из Redis + сбрасывает кеш профиля.
func (s *AuthService) LogoutAll(userID uint) error {
	if err := s.DB.Model(&models.Token{}).
		Where("user_id = ? AND revoked = ?", userID, false).
		Update("revoked", true).Error; err != nil {
		return err
	}

	if s.Cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		// Все access-JTI текущего юзера: wp:auth:user:{id}:access:*
		if err := s.Cache.DelByPattern(ctx, cache.KeyAccessJTIPatternForUser(userID)); err != nil {
			log.Printf("[auth] LogoutAll: ошибка очистки access JTI: %v", err)
		}
		_ = s.Cache.Del(ctx, cache.KeyUserProfile(userID))
	}
	return nil
}

// GetUserByID — получение профиля для /whoami.
// Cache-Aside: пробуем Redis, при miss — БД + запись в кеш.
func (s *AuthService) GetUserByID(ctx context.Context, id uint) (*models.User, error) {
	key := cache.KeyUserProfile(id)

	// 1. Кеш.
	if s.Cache != nil {
		var cached models.User
		if err := s.Cache.Get(ctx, key, &cached); err == nil {
			log.Printf("[auth] HIT  %s", key)
			return &cached, nil
		}
	}

	// 2. БД.
	log.Printf("[auth] MISS %s — запрос в БД", key)
	var user models.User
	if err := s.DB.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	// 3. Запись в кеш.
	// Внимание: модель User содержит PasswordHash/Salt. JSON-маршалинг пропускает их
	// благодаря тегу `json:"-"` на полях модели (см. models/user.go) — пароль и соль
	// в Redis НЕ попадают, что соответствует требованию задания.
	if s.Cache != nil {
		_ = s.Cache.Set(ctx, key, user, 0)
	}
	return &user, nil
}

// IsAccessJTIValid — проверка в Redis, что JTI access-токена ещё активен.
// Если Redis недоступен — возвращаем true (fail-open): JWT-подпись уже валидна,
// мы просто не смогли проверить отзыв. Это компромисс: альтернатива (fail-closed) положила бы
// весь сервис при падении Redis. Для строгих сценариев это решение можно поменять.
func (s *AuthService) IsAccessJTIValid(ctx context.Context, userID uint, jti string) bool {
	if s.Cache == nil || !s.Cache.IsEnabled() {
		return true
	}
	ok, err := s.Cache.Exists(ctx, cache.KeyAccessJTI(userID, jti))
	if err != nil {
		// Redis отвалился на лету — fail-open.
		return true
	}
	return ok
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
