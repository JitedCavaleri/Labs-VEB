package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenType различает access и refresh токены — у них разные секреты и срок жизни.
type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

// Claims — кастомные claims нашего JWT.
//
// Лаба 6: UserID теперь string, а не uint. MongoDB-шный _id — это ObjectID,
// и в claims мы кладём его hex-представление (24 hex символа). На приёме
// сервисы парсят его обратно в primitive.ObjectID.
type Claims struct {
	UserID string    `json:"user_id"`
	Type   TokenType `json:"type"`
	jwt.RegisteredClaims
}

// GenerateToken создаёт подписанный JWT с заданным сроком жизни.
// Возвращает строку токена и JTI (для сохранения в БД).
func GenerateToken(userID string, tokenType TokenType, secret string, ttl time.Duration) (string, string, error) {
	jti := uuid.NewString()
	claims := Claims{
		UserID: userID,
		Type:   tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", "", err
	}
	return signed, jti, nil
}

// ParseToken валидирует подпись и срок действия.
// Также проверяет, что тип токена соответствует ожидаемому
// (нельзя подменить access на refresh).
func ParseToken(tokenString, secret string, expectedType TokenType) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		// Защита от подмены алгоритма (например, на "none").
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("неожиданный метод подписи")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("невалидный токен")
	}
	if claims.Type != expectedType {
		return nil, errors.New("неверный тип токена")
	}
	return claims, nil
}

// HashToken возвращает SHA-256 хеш токена в hex.
// Используется при сохранении refresh-токенов в БД — сам токен не хранится.
// SHA-256 здесь достаточно: токен уже сам по себе длинная случайная строка
// (благодаря JTI/UUID), поэтому атака по словарю невозможна, в отличие от паролей.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
