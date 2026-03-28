package utils

import (
	"awesomeProject/config"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SetAuthCookies — устанавливает Access и Refresh токены в HttpOnly cookies
func SetAuthCookies(c *gin.Context, accessToken, refreshToken string) {
	// Access Token (короткий, 15 минут)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     "/",
		MaxAge:   int(config.JWT.AccessExpiration.Seconds()),
		HttpOnly: true,                    // JavaScript не сможет прочитать
		Secure:   false,                   // false — для локальной разработки (в проде true + HTTPS)
		SameSite: http.SameSiteStrictMode, // защита от CSRF
	})

	// Refresh Token (долгий, 7 дней)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		MaxAge:   int(config.JWT.RefreshExpiration.Seconds()),
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearAuthCookies — полностью удаляет cookies при logout
func ClearAuthCookies(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // сразу удаляется
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	})

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	})
}
