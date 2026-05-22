package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"strings"
	"time"

	"awesomeProject/database"
	"awesomeProject/dto"
	"awesomeProject/models"
	"awesomeProject/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// PasswordResetService — реализация "forgot password" / "reset password" потока.
//
// Лаба 6: вместо PostgreSQL/GORM работа с MongoDB. Поскольку MongoDB Community
// поддерживает transactions только на replica set / sharded cluster, в одиночном
// инстансе (как в нашем docker-compose) мы выполняем 3 update'a последовательно
// без транзакции. В реальном продакшене это можно завернуть в Mongo transaction,
// если БД развёрнута как replica set.
type PasswordResetService struct {
	DB *database.DB
}

func NewPasswordResetService(db *database.DB) *PasswordResetService {
	return &PasswordResetService{DB: db}
}

func (s *PasswordResetService) usersColl() *mongo.Collection {
	return s.DB.Mongo.Collection(database.CollUsers)
}
func (s *PasswordResetService) prtColl() *mongo.Collection {
	return s.DB.Mongo.Collection(database.CollPasswordResetTokens)
}
func (s *PasswordResetService) tokensColl() *mongo.Collection {
	return s.DB.Mongo.Collection(database.CollTokens)
}

// CreateResetToken — генерирует reset-токен и сохраняет его хеш в БД.
// В реальном приложении отправляется на email; здесь логируется (для лабы).
func (s *PasswordResetService) CreateResetToken(req dto.ForgotPasswordRequest) error {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := s.usersColl().FindOne(ctx, database.MergeAlive(bson.M{"email": email})).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
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
		ID:        primitive.NewObjectID(),
		UserID:    user.ID,
		TokenHash: utils.HashToken(rawToken),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Used:      false,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := s.prtColl().InsertOne(ctx, record); err != nil {
		return err
	}

	// В реальной системе — отправить email с этим токеном.
	// В логе показываем ТОЛЬКО первые 8 символов, чтобы не утекал в логи.
	preview := rawToken
	if len(preview) > 8 {
		preview = preview[:8] + "..."
	}
	log.Printf("[password-reset] токен для user_id=%s создан (preview=%s)", user.ID.Hex(), preview)
	log.Printf("[password-reset] FULL TOKEN (только для отладки лабы): %s", rawToken)

	return nil
}

// ResetPassword — устанавливает новый пароль по reset-токену.
//
// Лаба 6: операции не обёрнуты в Mongo transaction (требует replica set).
// Порядок выбран так, чтобы при сбое середины не получить уязвимое состояние:
//  1. Поднимаем пароль — у пользователя сразу новый pw.
//  2. Помечаем reset-токен использованным — защита от replay.
//  3. Отзываем все refresh-токены — старые сессии умирают.
//
// Если шаги 2/3 упадут — пароль уже новый, репит можно повторить.
func (s *PasswordResetService) ResetPassword(req dto.ResetPasswordRequest) error {
	hash := utils.HashToken(req.Token)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var record models.PasswordResetToken
	if err := s.prtColl().FindOne(ctx, bson.M{"tokenHash": hash}).Decode(&record); err != nil {
		return ErrInvalidToken
	}
	if record.Used || record.ExpiresAt.Before(time.Now()) {
		return ErrInvalidToken
	}

	if err := validatePasswordStrength(req.NewPassword); err != nil {
		return err
	}

	var user models.User
	if err := s.usersColl().FindOne(ctx, database.MergeAlive(bson.M{"_id": record.UserID})).Decode(&user); err != nil {
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

	now := time.Now().UTC()

	// 1. Обновляем пароль.
	if _, err := s.usersColl().UpdateByID(ctx, user.ID, bson.M{
		"$set": bson.M{
			"passwordHash": newHash,
			"salt":         newSalt,
			"updatedAt":    now,
		},
	}); err != nil {
		return err
	}

	// 2. Помечаем reset-токен использованным.
	if _, err := s.prtColl().UpdateByID(ctx, record.ID, bson.M{
		"$set": bson.M{"used": true},
	}); err != nil {
		return err
	}

	// 3. Отзываем все активные refresh-токены пользователя.
	if _, err := s.tokensColl().UpdateMany(ctx,
		bson.M{"userId": user.ID, "revoked": false},
		bson.M{"$set": bson.M{"revoked": true, "updatedAt": now}},
	); err != nil {
		return err
	}

	return nil
}
