package main

import (
	"log"

	"awesomeProject/cache"
	"awesomeProject/config"
	"awesomeProject/database"
	_ "awesomeProject/docs" // сгенерированный swag init пакет — обязательный blank-import
	"awesomeProject/handlers"
	"awesomeProject/middleware"
	"awesomeProject/services"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title           Lab Project API
// @version         1.0
// @description     REST API для лабораторных работ №2–№6 (Web-Programming).
// @description     Включает: публичный /info, регистрацию/вход с JWT в HttpOnly cookies,
// @description     OAuth 2.0 через Yandex и VK, CRUD ресурсов с проверкой владения, Soft Delete.
// @description     Лаба 5 добавляет кеширование в Redis: списки ресурсов, профиль пользователя,
// @description     реестр валидных access-JTI для мгновенного отзыва токенов.
// @description     Лаба 6 заменяет PostgreSQL на MongoDB: вместо GORM используется
// @description     официальный mongo-driver, идентификаторы — ObjectID (24 hex символа),
// @description     soft-delete реализован через поле deletedAt на уровне приложения.
// @description
// @description     ВАЖНО: реальное приложение использует HttpOnly cookies (access_token / refresh_token).
// @description     В Swagger UI для удобства тестирования также описана схема BearerAuth — её можно использовать,
// @description     если запустить локально приложение, в котором временно отключён HttpOnly (для отладки).

// @contact.name  Lab Author
// @contact.url   https://github.com/

// @license.name  MIT

// @host       localhost:4200
// @BasePath   /
// @schemes    http https

// @tag.name         Public
// @tag.description  Открытые эндпоинты, не требующие авторизации
// @tag.name         Auth
// @tag.description  Регистрация, вход, выход, сброс пароля и /whoami
// @tag.name         OAuth
// @tag.description  OAuth 2.0 (Authorization Code Grant) через Yandex и VK
// @tag.name         Items
// @tag.description  CRUD пользовательских ресурсов (защищено)

// @securityDefinitions.apikey  CookieAuth
// @in                          cookie
// @name                        access_token
// @description                 JWT access-токен, хранится в HttpOnly cookie. Браузер прикрепит его автоматически после /auth/login или /auth/register.

// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 Альтернативный способ передачи JWT (в отладочных целях): `Bearer <access_token>`. В реальном приложении используется CookieAuth.

// @securitydefinitions.oauth2.accessCode  YandexOAuth
// @tokenUrl                               https://oauth.yandex.ru/token
// @authorizationUrl                       https://oauth.yandex.ru/authorize
// @scope.login:email                      Email пользователя
// @scope.login:info                       Базовая информация о пользователе
// @description                            Authorization Code Grant через Яндекс. Реализован вручную в OAuthService (см. /auth/oauth/yandex).

// @securitydefinitions.oauth2.accessCode  VkOAuth
// @tokenUrl                               https://oauth.vk.com/access_token
// @authorizationUrl                       https://oauth.vk.com/authorize
// @scope.email                            Email пользователя
// @description                            Authorization Code Grant через VK. Реализован вручную (см. /auth/oauth/vk).

func main() {
	// godotenv не падает, если файла нет — в docker-compose env_file делает то же самое.
	_ = godotenv.Load()

	cfg := config.LoadConfig()

	// --- Лаба 6: MongoDB вместо PostgreSQL ---
	// database.Connect возвращает обёртку *database.DB с *mongo.Client + *mongo.Database.
	db := database.Connect()
	defer db.Disconnect()

	// "Миграция" в MongoDB — это создание индексов (uniq email, индексы по userId/jti/tokenHash и т.д.).
	if err := database.Migrate(db); err != nil {
		log.Fatalf("Ошибка миграции (создания индексов): %v", err)
	}

	// --- Лаба 5: кеш-сервис (Redis) ---
	// New не падает, если Redis недоступен — внутри логируется,
	// и приложение продолжает работать "без кеша" (graceful degradation).
	cacheSvc := cache.New(cache.Config{
		Addr:       cfg.RedisAddr,
		Password:   cfg.RedisPassword,
		DB:         cfg.RedisDB,
		DefaultTTL: cfg.CacheTTLDefault,
	})
	defer func() { _ = cacheSvc.Close() }()

	// Сервисный слой
	authSvc := services.NewAuthService(db, cfg, cacheSvc)
	oauthSvc := services.NewOAuthService(db, cfg)
	passwordResetSvc := services.NewPasswordResetService(db)
	itemSvc := services.NewItemService(db, cacheSvc)

	// Хендлеры
	authHandler := &handlers.AuthHandler{
		AuthSvc:          authSvc,
		OAuthSvc:         oauthSvc,
		PasswordResetSvc: passwordResetSvc,
		Cfg:              cfg,
	}
	itemHandler := &handlers.ItemHandler{Service: itemSvc}

	r := gin.Default()

	// Публичный эндпоинт из лабы 2
	r.GET("/info", handlers.Info)

	// ---- Публичные маршруты авторизации ----
	auth := r.Group("/auth")
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
		auth.POST("/forgot-password", authHandler.ForgotPassword)
		auth.POST("/reset-password", authHandler.ResetPassword)

		// OAuth — Public (302 Redirect)
		auth.GET("/oauth/:provider", authHandler.OAuthInit)
		auth.GET("/oauth/:provider/callback", authHandler.OAuthCallback)
	}

	// ---- Приватные маршруты авторизации ----
	// Лаба 5: middleware теперь зависит и от authSvc — чтобы сверять JTI access-токена с Redis.
	authPrivate := r.Group("/auth")
	authPrivate.Use(middleware.AuthMiddleware(cfg, authSvc))
	{
		authPrivate.GET("/whoami", authHandler.WhoAmI)
		authPrivate.POST("/logout", authHandler.Logout)
		authPrivate.POST("/logout-all", authHandler.LogoutAll)
	}

	// ---- Защищённый CRUD ресурсов (из лабы 2) ----
	items := r.Group("/items")
	items.Use(middleware.AuthMiddleware(cfg, authSvc))
	{
		items.POST("", itemHandler.Create)
		items.GET("", itemHandler.GetAll)
		items.GET("/:id", itemHandler.GetByID)
		items.PUT("/:id", itemHandler.Update)
		items.PATCH("/:id", itemHandler.Patch)
		items.DELETE("/:id", itemHandler.Delete)
	}

	// ---- Swagger UI: только вне production ----
	// В production маршрут /api/docs физически не регистрируется,
	// поэтому Gin вернёт 404 — что и требуется по заданию лабы 4.
	if !cfg.IsProduction() {
		r.GET("/api/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
		// Удобный редирект: /api/docs -> /api/docs/index.html
		r.GET("/api/docs", func(c *gin.Context) {
			c.Redirect(302, "/api/docs/index.html")
		})
		log.Printf("Swagger UI доступен по адресу: http://localhost:%s/api/docs/index.html", cfg.AppPort)
	} else {
		log.Println("APP_ENV=production: Swagger UI отключён, /api/docs вернёт 404")
	}

	log.Printf("Сервер запущен на :%s (env=%s, cache=%v)", cfg.AppPort, cfg.AppEnv, cacheSvc.IsEnabled())
	if err := r.Run(":" + cfg.AppPort); err != nil {
		log.Fatalf("Сервер упал: %v", err)
	}
}
