package middleware

import (
	"net/http"

	"awesomeProject/config"
	"awesomeProject/services"
	"awesomeProject/utils"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware — Guard для защищённых маршрутов.
//
// Шаги:
//  1. Извлекаем access_token из HttpOnly cookie (или из заголовка Authorization).
//  2. Проверяем подпись и срок действия JWT.
//  3. (Лаба 5) Проверяем, что JTI этого токена есть в Redis-реестре. Если нет —
//     токен был отозван через /auth/logout раньше срока, доступ запрещён.
//
// Если Redis недоступен, шаг 3 пропускается (fail-open) — см. IsAccessJTIValid.
//
// В gin.Context кладём:
//   - user_id     (uint)   — для хендлеров
//   - access_jti  (string) — нужен Logout, чтобы знать, какой JTI удалять
func AuthMiddleware(cfg *config.Config, authSvc *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := extractToken(c)
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Требуется авторизация"})
			return
		}

		claims, err := utils.ParseToken(tokenString, cfg.JWTAccessSecret, utils.AccessToken)
		if err != nil {
			// Не отдаём клиенту техническую причину (истёк / невалидная подпись / неверный тип),
			// чтобы не помогать злоумышленнику в перебирании сценариев.
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Невалидный или истёкший токен"})
			return
		}

		// --- Лаба 5: проверка JTI в Redis-реестре. ---
		// Если ключа нет (например, был /auth/logout) — отказываем,
		// даже если JWT-подпись формально валидна.
		if !authSvc.IsAccessJTIValid(c.Request.Context(), claims.UserID, claims.ID) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Сессия отозвана"})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("access_jti", claims.ID)
		c.Next()
	}
}

// extractToken — читает токен из cookie или из заголовка Authorization: Bearer ...
// Cookie приоритетнее (как в лабе 3/4), Authorization оставлен для отладки/Swagger.
func extractToken(c *gin.Context) string {
	if tok, err := c.Cookie("access_token"); err == nil && tok != "" {
		return tok
	}
	authHeader := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if len(authHeader) > len(prefix) && authHeader[:len(prefix)] == prefix {
		return authHeader[len(prefix):]
	}
	return ""
}

// GetUserID извлекает user_id, положенный middleware'ом.
// Используется в хендлерах защищённых маршрутов.
func GetUserID(c *gin.Context) (uint, bool) {
	v, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}

// GetAccessJTI — извлекает JTI access-токена из контекста (положен middleware'ом).
// Нужен в /auth/logout, чтобы корректно удалить ИМЕННО ТОТ JTI, по которому пришёл запрос.
func GetAccessJTI(c *gin.Context) string {
	v, exists := c.Get("access_jti")
	if !exists {
		return ""
	}
	s, _ := v.(string)
	return s
}
