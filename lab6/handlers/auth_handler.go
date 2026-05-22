package handlers

import (
	"errors"
	"net/http"

	"awesomeProject/config"
	"awesomeProject/database"
	"awesomeProject/dto"
	"awesomeProject/middleware"
	"awesomeProject/services"
	"awesomeProject/utils"

	"github.com/gin-gonic/gin"
)

// AuthHandler — обработчики маршрутов /auth/*.
type AuthHandler struct {
	AuthSvc          *services.AuthService
	OAuthSvc         *services.OAuthService
	PasswordResetSvc *services.PasswordResetService
	Cfg              *config.Config
}

// cookieCfg возвращает CookieConfig из конфига приложения.
func (h *AuthHandler) cookieCfg() utils.CookieConfig {
	return utils.CookieConfig{
		Domain: h.Cfg.CookieDomain,
		Secure: h.Cfg.CookieSecure,
	}
}

// setAuthCookies — ставит access и refresh cookies сразу.
func (h *AuthHandler) setAuthCookies(c *gin.Context, access, refresh string) {
	cfg := h.cookieCfg()
	utils.SetAuthCookie(c, "access_token", access, int(h.Cfg.JWTAccessExpiration.Seconds()), cfg)
	utils.SetAuthCookie(c, "refresh_token", refresh, int(h.Cfg.JWTRefreshExpiration.Seconds()), cfg)
}

// Register godoc
// @Summary      Регистрация нового пользователя
// @Description  Создаёт пользователя и сразу выдаёт пару JWT-токенов в HttpOnly cookies (access_token, refresh_token).
// @Description  Пароль должен содержать минимум 8 символов, обязательно буквы и цифры.
// @Description  Лаба 5: JTI выпущенного access-токена регистрируется в Redis для возможности отзыва.
// @Description  Лаба 6: пользователь хранится в MongoDB коллекции users; _id — ObjectID.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        body  body      dto.RegisterRequest  true  "Данные регистрации"
// @Success      201   {object}  dto.AuthSuccessResponse
// @Failure      400   {object}  dto.ErrorResponse "Неверные данные регистрации или слабый пароль"
// @Failure      409   {object}  dto.ErrorResponse "Пользователь с таким email уже существует"
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверные данные регистрации"})
		return
	}

	user, err := h.AuthSvc.Register(req)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrEmailAlreadyExists):
			c.JSON(http.StatusConflict, dto.ErrorResponse{Error: err.Error()})
		case errors.Is(err, services.ErrWeakPassword):
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Не удалось создать пользователя"})
		}
		return
	}

	access, refresh, err := h.AuthSvc.IssueTokens(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Не удалось выпустить токены"})
		return
	}
	h.setAuthCookies(c, access, refresh)

	c.JSON(http.StatusCreated, dto.AuthSuccessResponse{
		Message: "Пользователь зарегистрирован",
		User:    services.ToProfileResponse(user),
	})
}

// Login godoc
// @Summary      Вход в систему
// @Description  Проверяет email/пароль и устанавливает HttpOnly cookies access_token и refresh_token.
// @Description  В целях безопасности (защита от user enumeration) на любую неверную пару возвращает 401.
// @Description  Лаба 5: JTI выпущенного access-токена сохраняется в Redis с TTL = времени жизни access.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        body  body      dto.LoginRequest  true  "Учётные данные"
// @Success      200   {object}  dto.AuthSuccessResponse
// @Failure      400   {object}  dto.ErrorResponse "Неверные данные входа"
// @Failure      401   {object}  dto.ErrorResponse "Неверный email или пароль"
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверные данные входа"})
		return
	}

	user, err := h.AuthSvc.Login(req)
	if err != nil {
		// 401 для любой проблемы с учётными данными (без раскрытия деталей).
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: services.ErrInvalidCredentials.Error()})
		return
	}

	access, refresh, err := h.AuthSvc.IssueTokens(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Не удалось выпустить токены"})
		return
	}
	h.setAuthCookies(c, access, refresh)

	c.JSON(http.StatusOK, dto.AuthSuccessResponse{
		Message: "Успешный вход",
		User:    services.ToProfileResponse(user),
	})
}

// Refresh godoc
// @Summary      Обновить пару токенов
// @Description  Читает refresh_token из HttpOnly cookie, отзывает старый и выдаёт новую пару access/refresh.
// @Description  Реализована refresh-rotation: после обмена старый refresh становится невалидным.
// @Tags         Auth
// @Produce      json
// @Success      200   {object}  dto.MessageResponse
// @Failure      401   {object}  dto.ErrorResponse "Refresh-токен отсутствует или невалиден"
// @Router       /auth/refresh [post]
func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshTok, err := c.Cookie("refresh_token")
	if err != nil || refreshTok == "" {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Refresh-токен отсутствует"})
		return
	}

	newAccess, newRefresh, err := h.AuthSvc.Refresh(refreshTok)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Не удалось обновить токены"})
		return
	}
	h.setAuthCookies(c, newAccess, newRefresh)

	c.JSON(http.StatusOK, dto.MessageResponse{Message: "Токены обновлены"})
}

// WhoAmI godoc
// @Summary      Получить профиль текущего пользователя
// @Description  Возвращает безопасную проекцию пользователя (без пароля/соли/идентификаторов OAuth).
// @Description  Требует валидного access_token в cookie.
// @Description  Лаба 5: профиль кешируется в Redis под ключом wp:users:profile:{userId}.
// @Description  Лаба 6: userId теперь hex от MongoDB ObjectID.
// @Tags         Auth
// @Produce      json
// @Success      200   {object}  dto.UserProfileResponse
// @Failure      401   {object}  dto.ErrorResponse "Не авторизован / токен невалиден"
// @Security     CookieAuth
// @Security     BearerAuth
// @Router       /auth/whoami [get]
func (h *AuthHandler) WhoAmI(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Не авторизован"})
		return
	}
	user, err := h.AuthSvc.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) || database.IsNotFound(err) {
			c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Пользователь не найден"})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Ошибка получения профиля"})
		return
	}
	c.JSON(http.StatusOK, services.ToProfileResponse(user))
}

// Logout godoc
// @Summary      Выход (только текущая сессия)
// @Description  Отзывает текущий refresh-токен и очищает cookies. Остальные сессии пользователя остаются активными.
// @Description  Лаба 5: удаляет JTI текущего access-токена из Redis (мгновенный отзыв доступа)
// @Description  и сбрасывает кеш профиля.
// @Tags         Auth
// @Produce      json
// @Success      200   {object}  dto.MessageResponse
// @Failure      401   {object}  dto.ErrorResponse "Не авторизован"
// @Security     CookieAuth
// @Security     BearerAuth
// @Router       /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	refreshTok, _ := c.Cookie("refresh_token")
	userID, _ := middleware.GetUserID(c)
	accessJTI := middleware.GetAccessJTI(c)

	_ = h.AuthSvc.Logout(refreshTok, userID, accessJTI)

	cfg := h.cookieCfg()
	utils.ClearAuthCookie(c, "access_token", cfg)
	utils.ClearAuthCookie(c, "refresh_token", cfg)

	c.JSON(http.StatusOK, dto.MessageResponse{Message: "Выход выполнен"})
}

// LogoutAll godoc
// @Summary      Выход со всех устройств
// @Description  Отзывает ВСЕ активные refresh-токены пользователя — например, при подозрении на компрометацию аккаунта.
// @Description  Лаба 5: дополнительно удаляет ВСЕ access-JTI пользователя из Redis (по паттерну wp:auth:user:{id}:access:*)
// @Description  и сбрасывает кеш профиля.
// @Tags         Auth
// @Produce      json
// @Success      200   {object}  dto.MessageResponse
// @Failure      401   {object}  dto.ErrorResponse "Не авторизован"
// @Failure      500   {object}  dto.ErrorResponse
// @Security     CookieAuth
// @Security     BearerAuth
// @Router       /auth/logout-all [post]
func (h *AuthHandler) LogoutAll(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Не авторизован"})
		return
	}
	if err := h.AuthSvc.LogoutAll(userID); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Не удалось завершить сессии"})
		return
	}

	cfg := h.cookieCfg()
	utils.ClearAuthCookie(c, "access_token", cfg)
	utils.ClearAuthCookie(c, "refresh_token", cfg)

	c.JSON(http.StatusOK, dto.MessageResponse{Message: "Все сессии завершены"})
}

// OAuthInit godoc
// @Summary      Начать OAuth-вход через провайдера
// @Description  Перенаправляет (302 Found) на страницу авторизации провайдера (yandex или vk).
// @Description  Реализован Authorization Code Grant с защитой через одноразовый параметр state (CSRF-protection).
// @Tags         OAuth
// @Param        provider  path  string  true  "Провайдер: yandex | vk"  Enums(yandex, vk)
// @Success      302       "Redirect to provider"
// @Failure      400       {object}  dto.ErrorResponse  "Неизвестный или не настроенный провайдер"
// @Router       /auth/oauth/{provider} [get]
func (h *AuthHandler) OAuthInit(c *gin.Context) {
	provider := c.Param("provider")
	authURL, err := h.OAuthSvc.BuildAuthURL(provider)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	c.Redirect(http.StatusFound, authURL)
}

// OAuthCallback godoc
// @Summary      Callback OAuth-провайдера
// @Description  Принимает code/state от провайдера, обменивает code на access_token провайдера,
// @Description  получает данные пользователя и логинит/регистрирует его. Ставит JWT cookies.
// @Tags         OAuth
// @Produce      json
// @Param        provider  path   string  true   "Провайдер: yandex | vk"  Enums(yandex, vk)
// @Param        code      query  string  true   "Authorization code от провайдера"
// @Param        state     query  string  true   "Anti-CSRF state, ранее выданный сервером"
// @Success      200       {object}  dto.AuthSuccessResponse
// @Failure      400       {object}  dto.ErrorResponse "Отсутствуют code или state"
// @Failure      401       {object}  dto.ErrorResponse "Не удалось завершить OAuth"
// @Failure      500       {object}  dto.ErrorResponse
// @Router       /auth/oauth/{provider}/callback [get]
func (h *AuthHandler) OAuthCallback(c *gin.Context) {
	provider := c.Param("provider")
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Отсутствуют параметры code или state"})
		return
	}

	user, err := h.OAuthSvc.HandleCallback(provider, code, state)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Не удалось завершить OAuth: " + err.Error()})
		return
	}

	access, refresh, err := h.AuthSvc.IssueTokens(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Не удалось выпустить токены"})
		return
	}
	h.setAuthCookies(c, access, refresh)

	// В реальном приложении — редирект на фронтенд.
	// Для лабы — отдаём JSON с профилем.
	c.JSON(http.StatusOK, dto.AuthSuccessResponse{
		Message: "Вход через OAuth выполнен",
		User:    services.ToProfileResponse(user),
	})
}

// ForgotPassword godoc
// @Summary      Запросить сброс пароля
// @Description  Принимает email; если пользователь существует — генерирует токен сброса (отправляется на email; в лабе пишется в лог).
// @Description  Эндпоинт ВСЕГДА отвечает 200 — это защита от user enumeration.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        body  body      dto.ForgotPasswordRequest  true  "Email"
// @Success      200   {object}  dto.MessageResponse
// @Failure      400   {object}  dto.ErrorResponse "Неверный email"
// @Router       /auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req dto.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверный email"})
		return
	}
	// Намеренно всегда отвечаем одинаково — защита от user enumeration.
	_ = h.PasswordResetSvc.CreateResetToken(req)
	c.JSON(http.StatusOK, dto.MessageResponse{Message: "Если такой email зарегистрирован, на него отправлены инструкции"})
}

// ResetPassword godoc
// @Summary      Установить новый пароль по reset-токену
// @Description  Принимает токен (из письма) и новый пароль. Все активные сессии отзываются после смены пароля.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        body  body      dto.ResetPasswordRequest  true  "Reset-токен и новый пароль"
// @Success      200   {object}  dto.MessageResponse
// @Failure      400   {object}  dto.ErrorResponse "Неверные данные / невалидный или истёкший токен / слабый пароль"
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /auth/reset-password [post]
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req dto.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверные данные"})
		return
	}

	if err := h.PasswordResetSvc.ResetPassword(req); err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidToken):
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Невалидный или истёкший токен"})
		case errors.Is(err, services.ErrWeakPassword):
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Не удалось сбросить пароль"})
		}
		return
	}
	c.JSON(http.StatusOK, dto.MessageResponse{Message: "Пароль успешно изменён"})
}
