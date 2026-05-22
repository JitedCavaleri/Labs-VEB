package middleware

import (
	"net/http"

	"awesomeProject/config"
	"awesomeProject/utils"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware — Guard для защищённых маршрутов.
// Извлекает access_token из HttpOnly cookie, проверяет подпись и срок действия.
// При успехе кладёт user_id в gin.Context, иначе возвращает 401.
func AuthMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := c.Cookie("access_token")
		if err != nil || tokenString == "" {
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

		c.Set("user_id", claims.UserID)
		c.Next()
	}
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
