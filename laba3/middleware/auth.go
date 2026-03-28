package middleware

import (
	"net/http"

	"awesomeProject/config"
	"awesomeProject/utils"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie("access_token")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Токен доступа не найден"})
			c.Abort()
			return
		}

		claims, err := utils.ParseToken(cookie, config.JWT.AccessSecret)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Недействительный или просроченный access token"})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Next()
	}
}
