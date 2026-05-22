package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CookieConfig — параметры безопасности для cookie.
type CookieConfig struct {
	Domain string
	Secure bool
}

// SetAuthCookie ставит cookie с access или refresh токеном.
// Флаги:
//   - HttpOnly=true   — недоступна из JavaScript (защита от XSS-кражи)
//   - SameSite=Lax    — защита от CSRF (в проде можно усилить до Strict)
//   - Secure          — только по HTTPS (в проде true)
//   - Path=/          — отправлять на все эндпоинты
func SetAuthCookie(c *gin.Context, name, value string, maxAgeSeconds int, cfg CookieConfig) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		name,            // name
		value,           // value
		maxAgeSeconds,   // maxAge (секунды)
		"/",             // path
		cfg.Domain,      // domain
		cfg.Secure,      // secure
		true,            // httpOnly
	)
}

// ClearAuthCookie удаляет cookie (через MaxAge=-1).
func ClearAuthCookie(c *gin.Context, name string, cfg CookieConfig) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, "", -1, "/", cfg.Domain, cfg.Secure, true)
}
