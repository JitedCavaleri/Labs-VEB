package utils

import (
	"crypto/rand"
	"encoding/base64"

	"golang.org/x/crypto/bcrypt"
)

// GenerateSalt создаёт криптографически стойкую соль.
// Хотя bcrypt сам генерирует соль и встраивает её в хеш,
// согласно требованиям лабы хранить salt явно в отдельной колонке.
// Дополнительно используем salt как pepper при хешировании.
func GenerateSalt() (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(salt), nil
}

// HashPassword хеширует пароль bcrypt'ом, используя salt как pepper.
// bcrypt сам добавит свою внутреннюю соль (cost=12 — баланс скорости/безопасности).
func HashPassword(password, salt string) (string, error) {
	combined := password + salt
	hash, err := bcrypt.GenerateFromPassword([]byte(combined), 12)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword сравнивает пароль с сохранённым хешем.
func VerifyPassword(password, salt, hash string) bool {
	combined := password + salt
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(combined))
	return err == nil
}
