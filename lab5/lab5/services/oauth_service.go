package services

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"awesomeProject/config"
	"awesomeProject/models"

	"gorm.io/gorm"
)

// OAuthService — ручная реализация OAuth 2.0 Authorization Code Grant.
// Готовые библиотеки-обёртки (Goth, golang.org/x/oauth2/yandex) намеренно НЕ используются.
type OAuthService struct {
	DB  *gorm.DB
	Cfg *config.Config

	// stateStore — in-memory хранилище параметра state с TTL.
	// state защищает от CSRF при OAuth: callback с чужим/несуществующим state отвергается.
	stateStore sync.Map
}

func NewOAuthService(db *gorm.DB, cfg *config.Config) *OAuthService {
	return &OAuthService{DB: db, Cfg: cfg}
}

// providerEndpoints — параметры конкретных OAuth провайдеров.
type providerEndpoints struct {
	AuthURL     string
	TokenURL    string
	UserInfoURL string
	ClientID    string
	ClientSecret string
	RedirectURI string
	Scope       string
}

func (s *OAuthService) getEndpoints(provider string) (*providerEndpoints, error) {
	switch provider {
	case "yandex":
		return &providerEndpoints{
			AuthURL:      "https://oauth.yandex.ru/authorize",
			TokenURL:     "https://oauth.yandex.ru/token",
			UserInfoURL:  "https://login.yandex.ru/info?format=json",
			ClientID:     s.Cfg.YandexClientID,
			ClientSecret: s.Cfg.YandexClientSecret,
			RedirectURI:  s.Cfg.YandexCallbackURL,
			Scope:        "login:email login:info",
		}, nil
	case "vk":
		return &providerEndpoints{
			AuthURL:      "https://oauth.vk.com/authorize",
			TokenURL:     "https://oauth.vk.com/access_token",
			UserInfoURL:  "https://api.vk.com/method/users.get",
			ClientID:     s.Cfg.VKClientID,
			ClientSecret: s.Cfg.VKClientSecret,
			RedirectURI:  s.Cfg.VKCallbackURL,
			Scope:        "email",
		}, nil
	default:
		return nil, fmt.Errorf("неизвестный OAuth провайдер: %s", provider)
	}
}

// BuildAuthURL генерирует URL для редиректа на провайдера + сохраняет state.
func (s *OAuthService) BuildAuthURL(provider string) (string, error) {
	ep, err := s.getEndpoints(provider)
	if err != nil {
		return "", err
	}
	if ep.ClientID == "" {
		return "", fmt.Errorf("провайдер %s не настроен (отсутствует CLIENT_ID)", provider)
	}

	state, err := generateState()
	if err != nil {
		return "", err
	}
	// Сохраняем state с TTL в 10 минут.
	s.stateStore.Store(state, time.Now().Add(10*time.Minute))

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", ep.ClientID)
	q.Set("redirect_uri", ep.RedirectURI)
	q.Set("state", state)
	if ep.Scope != "" {
		q.Set("scope", ep.Scope)
	}

	return ep.AuthURL + "?" + q.Encode(), nil
}

// validateState проверяет state и сразу удаляет его (one-time use).
func (s *OAuthService) validateState(state string) error {
	val, ok := s.stateStore.LoadAndDelete(state)
	if !ok {
		return errors.New("неизвестный state — возможно CSRF атака")
	}
	expiresAt, ok := val.(time.Time)
	if !ok || time.Now().After(expiresAt) {
		return errors.New("state истёк")
	}
	return nil
}

// HandleCallback завершает Authorization Code Grant:
// проверяет state -> обменивает code на access_token провайдера ->
// получает данные пользователя -> ищет/создаёт локального User.
func (s *OAuthService) HandleCallback(provider, code, state string) (*models.User, error) {
	if err := s.validateState(state); err != nil {
		return nil, err
	}

	ep, err := s.getEndpoints(provider)
	if err != nil {
		return nil, err
	}

	// 1. Обмен кода на access_token провайдера.
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", ep.ClientID)
	form.Set("client_secret", ep.ClientSecret)
	form.Set("redirect_uri", ep.RedirectURI)

	req, err := http.NewRequest("POST", ep.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка обращения к провайдеру: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("провайдер вернул ошибку при обмене кода")
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Email       string `json:"email"` // VK иногда возвращает email прямо тут
		UserID      int    `json:"user_id"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("не удалось распарсить ответ провайдера")
	}
	if tokenResp.AccessToken == "" {
		return nil, errors.New("провайдер не вернул access_token")
	}

	// 2. Получение данных пользователя.
	providerID, email, err := s.fetchUserInfo(provider, tokenResp.AccessToken, tokenResp.Email, tokenResp.UserID)
	if err != nil {
		return nil, err
	}

	// 3. Поиск/создание локального пользователя.
	return s.findOrCreateOAuthUser(provider, providerID, email)
}

// fetchUserInfo обращается к userinfo-эндпоинту провайдера.
func (s *OAuthService) fetchUserInfo(provider, accessToken, vkEmail string, vkUserID int) (providerID, email string, err error) {
	ep, _ := s.getEndpoints(provider)

	switch provider {
	case "yandex":
		req, _ := http.NewRequest("GET", ep.UserInfoURL, nil)
		req.Header.Set("Authorization", "OAuth "+accessToken)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return "", "", err
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var info struct {
			ID          string `json:"id"`
			DefaultEmail string `json:"default_email"`
		}
		if err := json.Unmarshal(body, &info); err != nil {
			return "", "", err
		}
		if info.ID == "" {
			return "", "", errors.New("yandex не вернул id пользователя")
		}
		return info.ID, info.DefaultEmail, nil

	case "vk":
		// VK возвращает user_id и иногда email уже в ответе на token.
		q := url.Values{}
		q.Set("access_token", accessToken)
		q.Set("v", "5.131")
		q.Set("fields", "screen_name")

		resp, err := http.Get(ep.UserInfoURL + "?" + q.Encode())
		if err != nil {
			return "", "", err
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var info struct {
			Response []struct {
				ID int `json:"id"`
			} `json:"response"`
		}
		if err := json.Unmarshal(body, &info); err != nil || len(info.Response) == 0 {
			// fallback: используем user_id из token-ответа.
			if vkUserID == 0 {
				return "", "", errors.New("vk не вернул id пользователя")
			}
			return fmt.Sprintf("%d", vkUserID), vkEmail, nil
		}
		return fmt.Sprintf("%d", info.Response[0].ID), vkEmail, nil
	}
	return "", "", fmt.Errorf("неизвестный провайдер %s", provider)
}

// findOrCreateOAuthUser — ищет пользователя по provider_id, при отсутствии создаёт.
func (s *OAuthService) findOrCreateOAuthUser(provider, providerID, email string) (*models.User, error) {
	var user models.User
	var query *gorm.DB

	switch provider {
	case "yandex":
		query = s.DB.Where("yandex_id = ?", providerID)
	case "vk":
		query = s.DB.Where("vk_id = ?", providerID)
	}

	if err := query.First(&user).Error; err == nil {
		return &user, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Не нашли по provider_id — пробуем связать по email.
	if email != "" {
		if err := s.DB.Where("email = ?", strings.ToLower(email)).First(&user).Error; err == nil {
			// Привязываем provider к существующему аккаунту.
			switch provider {
			case "yandex":
				user.YandexID = providerID
			case "vk":
				user.VkID = providerID
			}
			s.DB.Save(&user)
			return &user, nil
		}
	}

	// Создаём нового пользователя без пароля (только OAuth).
	newUser := models.User{
		Email: strings.ToLower(email),
	}
	if email == "" {
		// Провайдер не отдал email — синтезируем уникальный.
		newUser.Email = fmt.Sprintf("%s_%s@oauth.local", provider, providerID)
	}
	switch provider {
	case "yandex":
		newUser.YandexID = providerID
	case "vk":
		newUser.VkID = providerID
	}
	if err := s.DB.Create(&newUser).Error; err != nil {
		return nil, err
	}
	return &newUser, nil
}

// generateState создаёт криптографически стойкую случайную строку.
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
