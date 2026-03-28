package handlers

import (
	"net/http"
	"net/url"

	"awesomeProject/config"
	"awesomeProject/dto"
	"awesomeProject/services"
	"awesomeProject/utils"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	AuthService *services.AuthService
}

// ====================== Обычная авторизация ======================

func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.UserCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.AuthService.Register(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось зарегистрировать"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":        user.ID,
		"email":     user.Email,
		"firstName": user.FirstName,
		"lastName":  user.LastName,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверные данные"})
		return
	}

	user, err := h.AuthService.Login(req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	access, refresh, _ := h.AuthService.CreateTokensAndSaveRefresh(user.ID)
	utils.SetAuthCookies(c, access, refresh)

	c.JSON(http.StatusOK, gin.H{"message": "Успешный вход"})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshCookie, err := c.Cookie("refresh_token")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token не найден"})
		return
	}

	access, refresh, err := h.AuthService.Refresh(refreshCookie)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	utils.SetAuthCookies(c, access, refresh)
	c.JSON(http.StatusOK, gin.H{"message": "Токены обновлены"})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	refreshCookie, _ := c.Cookie("refresh_token")
	if refreshCookie != "" {
		_ = h.AuthService.Logout(refreshCookie)
	}
	utils.ClearAuthCookies(c)
	c.JSON(http.StatusOK, gin.H{"message": "Вы вышли из системы"})
}

func (h *AuthHandler) LogoutAll(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Не авторизован"})
		return
	}

	_ = h.AuthService.LogoutAll(userID.(uint))
	utils.ClearAuthCookies(c)
	c.JSON(http.StatusOK, gin.H{"message": "Все сессии завершены"})
}

func (h *AuthHandler) Whoami(c *gin.Context) {
	userID, _ := c.Get("user_id")
	c.JSON(http.StatusOK, gin.H{"user_id": userID})
}

// ====================== OAuth Yandex ======================

func (h *AuthHandler) OAuthYandex(c *gin.Context) {
	state := utils.GenerateRandomState()

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
	})

	authURL := "https://oauth.yandex.ru/authorize?" + url.Values{
		"response_type": {"code"},
		"client_id":     {config.OAuth.YandexClientID},
		"redirect_uri":  {config.OAuth.YandexCallbackURL},
		"state":         {state},
	}.Encode()

	c.Redirect(http.StatusFound, authURL)
}

func (h *AuthHandler) OAuthYandexCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	// Проверка state (защита от CSRF)
	cookie, _ := c.Cookie("oauth_state")
	if cookie != state {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CSRF attack detected"})
		return
	}

	user, err := h.AuthService.OAuthYandexCallback(code) // ← теперь только code!
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка авторизации через Yandex"})
		return
	}

	access, refresh, _ := h.AuthService.CreateTokensAndSaveRefresh(user.ID)
	utils.SetAuthCookies(c, access, refresh)

	c.Redirect(http.StatusFound, "/")
}
