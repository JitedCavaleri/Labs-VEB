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
	"awesomeProject/database"
	"awesomeProject/dto"
	"awesomeProject/models"
	"awesomeProject/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// AuthService — бизнес-логика регистрации/входа/обновления токенов.
//
// Лаба 6: вместо *gorm.DB у сервиса теперь *database.DB (обёртка над *mongo.Database).
// Все запросы переписаны на MongoDB-driver. Soft Delete делаем вручную через поле
// deletedAt — драйвер этого "из коробки" не умеет, в отличие от GORM.
//
// Сохраняется логика лабы 5:
//  1. Реестр валидных access-токенов в Redis (по JTI). Позволяет мгновенно отозвать
//     access токен до его естественного истечения (см. /auth/logout).
//  2. Кеш профиля пользователя для /auth/whoami (cache-aside + инвалидация при logout).
type AuthService struct {
	DB    *database.DB
	Cfg   *config.Config
	Cache *cache.Service
}

func NewAuthService(db *database.DB, cfg *config.Config, c *cache.Service) *AuthService {
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

// usersColl / tokensColl — сокращения для удобства.
func (s *AuthService) usersColl() *mongo.Collection  { return s.DB.Mongo.Collection(database.CollUsers) }
func (s *AuthService) tokensColl() *mongo.Collection { return s.DB.Mongo.Collection(database.CollTokens) }

// Register создаёт нового пользователя с хешем пароля и уникальной солью.
func (s *AuthService) Register(req dto.RegisterRequest) (*models.User, error) {
	if err := validatePasswordStrength(req.Password); err != nil {
		return nil, err
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Проверка уникальности email (только среди "живых" пользователей).
	var existing models.User
	err := s.usersColl().FindOne(ctx, database.MergeAlive(bson.M{"email": email})).Decode(&existing)
	if err == nil {
		return nil, ErrEmailAlreadyExists
	}
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
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

	now := time.Now().UTC()
	user := models.User{
		ID:           primitive.NewObjectID(),
		Email:        email,
		Phone:        req.Phone,
		PasswordHash: hash,
		Salt:         salt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if _, err := s.usersColl().InsertOne(ctx, user); err != nil {
		// Гонка: если уникальный индекс отказался — другой запрос успел создать тот же email.
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrEmailAlreadyExists
		}
		return nil, err
	}
	return &user, nil
}

// Login проверяет учётные данные.
func (s *AuthService) Login(req dto.LoginRequest) (*models.User, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := s.usersColl().FindOne(ctx, database.MergeAlive(bson.M{"email": email})).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
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

// IssueTokens генерирует пару access+refresh, сохраняет хеш refresh в MongoDB
// и регистрирует JTI access-токена в Redis с TTL = время жизни access.
//
// JTI в Redis нужен, чтобы реализовать мгновенный Logout. JWT по своей природе stateless:
// без внешней проверки токен живёт до expiry. Запись JTI в Redis превращает access
// в "stateful" — middleware на каждом запросе сверяется с реестром.
//
// В кеш кладётся ТОЛЬКО идентификатор (JTI), а не сам токен — это требование безопасности
// из лабы 5: "Запрещено хранение полных токенов доступа в открытом виде".
func (s *AuthService) IssueTokens(userID primitive.ObjectID) (accessToken, refreshToken string, err error) {
	userHex := userID.Hex()

	accessToken, accessJTI, err := utils.GenerateToken(
		userHex, utils.AccessToken,
		s.Cfg.JWTAccessSecret, s.Cfg.JWTAccessExpiration,
	)
	if err != nil {
		return "", "", err
	}

	refreshToken, refreshJTI, err := utils.GenerateToken(
		userHex, utils.RefreshToken,
		s.Cfg.JWTRefreshSecret, s.Cfg.JWTRefreshExpiration,
	)
	if err != nil {
		return "", "", err
	}

	now := time.Now().UTC()
	// 1. Refresh — в MongoDB (как в лабе 3, но коллекция вместо таблицы).
	//    Долгоживущий, точная отзываемость через документ.
	tokenRecord := models.Token{
		ID:        primitive.NewObjectID(),
		UserID:    userID,
		TokenHash: utils.HashToken(refreshToken),
		JTI:       refreshJTI,
		ExpiresAt: now.Add(s.Cfg.JWTRefreshExpiration),
		Revoked:   false,
		CreatedAt: now,
		UpdatedAt: now,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.tokensColl().InsertOne(ctx, tokenRecord); err != nil {
		return "", "", err
	}

	// 2. Access JTI — в Redis с TTL = время жизни access токена.
	// Когда TTL истечёт — ключ исчезнет автоматически, дополнительная очистка не нужна.
	if s.Cache != nil {
		rctx, rcancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer rcancel()
		if err := s.Cache.SetString(
			rctx,
			cache.KeyAccessJTI(userHex, accessJTI),
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Проверяем, что токен есть в БД и не отозван.
	filter := bson.M{
		"jti":       claims.ID,
		"tokenHash": utils.HashToken(refreshToken),
	}
	var record models.Token
	if err := s.tokensColl().FindOne(ctx, filter).Decode(&record); err != nil {
		return "", "", ErrInvalidToken
	}
	if record.Revoked || record.ExpiresAt.Before(time.Now()) {
		return "", "", ErrInvalidToken
	}

	// Refresh rotation: отзываем старый.
	upd := bson.M{"$set": bson.M{"revoked": true, "updatedAt": time.Now().UTC()}}
	if _, err := s.tokensColl().UpdateByID(ctx, record.ID, upd); err != nil {
		return "", "", err
	}

	return s.IssueTokens(record.UserID)
}

// Logout отзывает один конкретный refresh-токен (текущая сессия)
// + удаляет JTI access токена из Redis (мгновенный отзыв доступа).
// + сбрасывает кеш профиля.
//
// userIDHex и accessJTI приходят из middleware (они положены в gin.Context).
func (s *AuthService) Logout(refreshToken, userIDHex, accessJTI string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Отзыв refresh в БД.
	if refreshToken != "" {
		hash := utils.HashToken(refreshToken)
		upd := bson.M{"$set": bson.M{"revoked": true, "updatedAt": time.Now().UTC()}}
		if _, err := s.tokensColl().UpdateMany(ctx, bson.M{"tokenHash": hash}, upd); err != nil {
			return err
		}
	}

	// 2. Отзыв access JTI в Redis и инвалидация кеша профиля.
	if s.Cache != nil && userIDHex != "" {
		rctx, rcancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer rcancel()

		keys := []string{cache.KeyUserProfile(userIDHex)}
		if accessJTI != "" {
			keys = append(keys, cache.KeyAccessJTI(userIDHex, accessJTI))
		}
		if err := s.Cache.Del(rctx, keys...); err != nil {
			log.Printf("[auth] Logout: ошибка удаления ключей из Redis: %v", err)
		}
	}
	return nil
}

// LogoutAll отзывает все refresh-токены пользователя (logout со всех устройств)
// + удаляет ВСЕ JTI access-токенов пользователя из Redis + сбрасывает кеш профиля.
func (s *AuthService) LogoutAll(userIDHex string) error {
	userID, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		return ErrUserNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	upd := bson.M{"$set": bson.M{"revoked": true, "updatedAt": time.Now().UTC()}}
	if _, err := s.tokensColl().UpdateMany(ctx, bson.M{"userId": userID, "revoked": false}, upd); err != nil {
		return err
	}

	if s.Cache != nil {
		rctx, rcancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer rcancel()
		// Все access-JTI текущего юзера: wp:auth:user:{id}:access:*
		if err := s.Cache.DelByPattern(rctx, cache.KeyAccessJTIPatternForUser(userIDHex)); err != nil {
			log.Printf("[auth] LogoutAll: ошибка очистки access JTI: %v", err)
		}
		_ = s.Cache.Del(rctx, cache.KeyUserProfile(userIDHex))
	}
	return nil
}

// GetUserByID — получение профиля для /whoami.
// Cache-Aside: пробуем Redis, при miss — БД + запись в кеш.
func (s *AuthService) GetUserByID(ctx context.Context, idHex string) (*models.User, error) {
	id, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return nil, ErrUserNotFound
	}
	key := cache.KeyUserProfile(idHex)

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
	err = s.usersColl().FindOne(ctx, database.MergeAlive(bson.M{"_id": id})).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	// 3. Запись в кеш.
	// Внимание: модель User содержит PasswordHash/Salt. JSON-маршалинг пропускает их
	// благодаря тегу `json:"-"` на полях модели — пароль и соль в Redis НЕ попадают.
	if s.Cache != nil {
		_ = s.Cache.Set(ctx, key, user, 0)
	}
	return &user, nil
}

// IsAccessJTIValid — проверка в Redis, что JTI access-токена ещё активен.
// Если Redis недоступен — возвращаем true (fail-open).
func (s *AuthService) IsAccessJTIValid(ctx context.Context, userIDHex, jti string) bool {
	if s.Cache == nil || !s.Cache.IsEnabled() {
		return true
	}
	ok, err := s.Cache.Exists(ctx, cache.KeyAccessJTI(userIDHex, jti))
	if err != nil {
		// Redis отвалился на лету — fail-open.
		return true
	}
	return ok
}

// ToProfileResponse — безопасная проекция модели в DTO без чувствительных полей.
func ToProfileResponse(u *models.User) dto.UserProfileResponse {
	return dto.UserProfileResponse{
		ID:        u.ID.Hex(),
		Email:     u.Email,
		Phone:     u.Phone,
		CreatedAt: u.CreatedAt,
	}
}
