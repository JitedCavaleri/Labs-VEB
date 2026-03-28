package services

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"awesomeProject/config"
	"awesomeProject/dto"
	"awesomeProject/models"
	"awesomeProject/utils"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"encoding/json"
)

type AuthService struct {
	DB          *gorm.DB
	UserService *UserService
}

func NewAuthService(db *gorm.DB, userService *UserService) *AuthService {
	return &AuthService{DB: db, UserService: userService}
}

// Register
func (s *AuthService) Register(req dto.UserCreateRequest) (models.User, error) {
	return s.UserService.Create(req)
}

// Login
func (s *AuthService) Login(req dto.LoginRequest) (models.User, error) {
	var user models.User
	if err := s.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		return models.User{}, errors.New("пользователь не найден")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return models.User{}, errors.New("неверный пароль")
	}

	return user, nil
}

// CreateTokensAndSaveRefresh — генерирует пару токенов + сохраняет Refresh в БД
func (s *AuthService) CreateTokensAndSaveRefresh(userID uint) (string, string, error) {
	accessToken, err := utils.GenerateAccessToken(userID)
	if err != nil {
		return "", "", err
	}
	refreshToken, err := utils.GenerateRefreshToken(userID)
	if err != nil {
		return "", "", err
	}

	// Хешируем Refresh-токен перед сохранением в БД
	tokenHash, _ := bcrypt.GenerateFromPassword([]byte(refreshToken), bcrypt.DefaultCost)

	tokenRecord := models.Token{
		UserID:    userID,
		TokenHash: string(tokenHash),
		ExpiresAt: time.Now().Add(config.JWT.RefreshExpiration), // ← исправлено
	}

	s.DB.Create(&tokenRecord)

	return accessToken, refreshToken, nil
}

// Refresh
func (s *AuthService) Refresh(refreshToken string) (string, string, error) {
	var tokenRecord models.Token
	if err := s.DB.Where("expires_at > ? AND revoked = false", time.Now()).
		First(&tokenRecord).Error; err != nil {
		return "", "", errors.New("refresh token недействителен или истёк")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(tokenRecord.TokenHash), []byte(refreshToken)); err != nil {
		return "", "", errors.New("refresh token недействителен")
	}

	return s.CreateTokensAndSaveRefresh(tokenRecord.UserID)
}

// Logout (текущая сессия)
func (s *AuthService) Logout(refreshToken string) error {
	var tokenRecord models.Token
	if err := s.DB.Where("token_hash = ?", refreshToken).First(&tokenRecord).Error; err != nil {
		return nil // если токен не найден — считаем, что уже logout
	}
	tokenRecord.Revoked = true
	return s.DB.Save(&tokenRecord).Error
}

// LogoutAll
func (s *AuthService) LogoutAll(userID uint) error {
	return s.DB.Model(&models.Token{}).
		Where("user_id = ? AND revoked = false", userID).
		Update("revoked", true).Error
}

// OAuthYandexCallback — полный поток вручную
func (s *AuthService) OAuthYandexCallback(code string) (models.User, error) {
	// 1. Обмен code на access_token Яндекса
	tokenURL := "https://oauth.yandex.ru/token"
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {config.OAuth.YandexClientID},
		"client_secret": {config.OAuth.YandexClientSecret},
		"redirect_uri":  {config.OAuth.YandexCallbackURL},
	}

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return models.User{}, err
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(resp.Body).Decode(&tokenResp)

	// 2. Получаем данные пользователя
	userInfoURL := "https://login.yandex.ru/info?oauth_token=" + tokenResp.AccessToken
	userResp, err := http.Get(userInfoURL)
	if err != nil {
		return models.User{}, err
	}
	defer userResp.Body.Close()

	var yandexUser struct {
		ID           string `json:"id"`
		DefaultEmail string `json:"default_email"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
	}
	json.NewDecoder(userResp.Body).Decode(&yandexUser)

	// 3. Ищем пользователя или создаём нового
	var user models.User
	err = s.DB.Where("yandex_id = ?", yandexUser.ID).First(&user).Error

	if err == gorm.ErrRecordNotFound {
		user = models.User{
			YandexID:  yandexUser.ID,
			Email:     yandexUser.DefaultEmail,
			FirstName: yandexUser.FirstName,
			LastName:  yandexUser.LastName,
		}
		s.DB.Create(&user)
	}

	return user, nil
}
