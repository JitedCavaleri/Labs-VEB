// Package cache — отдельный модуль работы с Redis.
//
// Лаба 5 требует выделить кеш в самостоятельный сервис и не разбрасывать вызовы Redis
// по всему коду. Здесь сосредоточено:
//
//   - подключение к Redis (с паролем из .env)
//   - сериализация значений в JSON (TTL обязателен для всех ключей)
//   - префиксная иерархическая схема ключей wp:module:entity:identifier
//   - graceful degradation: если Redis недоступен, приложение всё равно работает —
//     любая операция возвращает ErrCacheUnavailable, бизнес-логика откатывается на БД.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Префикс приложения. Все ключи лабы 5 начинаются с него.
// Использование префиксов — требование задания (раздел "Стандарты именования ключей").
const AppPrefix = "wp"

// ErrCacheUnavailable — кеш недоступен (нет коннекта, таймаут, ошибка протокола).
// Сервисы должны воспринимать это как cache miss, а не как ошибку запроса.
var ErrCacheUnavailable = errors.New("cache is unavailable")

// ErrCacheMiss — ключа нет в кеше. Семантически отличаем от ошибки соединения.
var ErrCacheMiss = errors.New("cache miss")

// Config — параметры подключения к Redis.
type Config struct {
	Addr       string        // host:port
	Password   string        // обязателен (см. требование к безопасности в задании)
	DB         int           // номер БД Redis (по умолчанию 0)
	DefaultTTL time.Duration // используется, если TTL не передан в Set
}

// Service — обёртка над go-redis с JSON-сериализацией.
// Все публичные методы безопасны при nil-клиенте: возвращают ErrCacheUnavailable.
type Service struct {
	client     *redis.Client
	defaultTTL time.Duration
	// enabled=false означает, что подключение к Redis при старте не удалось.
	// Все Get/Set будут no-op до перезапуска. Это сознательный выбор: лаба требует,
	// чтобы приложение НЕ падало при недоступности кеша.
	enabled bool
}

// New создаёт Service. PING делается единожды при старте.
// Ошибка подключения логируется, но не возвращается — приложение должно стартовать.
func New(cfg Config) *Service {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("[cache] Redis недоступен при старте: %v. Кеш будет отключён.", err)
		return &Service{client: client, defaultTTL: cfg.DefaultTTL, enabled: false}
	}

	log.Printf("[cache] Redis подключён: %s, prefix=%s, default TTL=%s",
		cfg.Addr, AppPrefix, cfg.DefaultTTL)

	return &Service{
		client:     client,
		defaultTTL: cfg.DefaultTTL,
		enabled:    true,
	}
}

// IsEnabled — для логов/healthcheck.
func (s *Service) IsEnabled() bool {
	return s != nil && s.enabled
}

// Close корректно закрывает соединение с Redis (вызывать перед выходом из main).
func (s *Service) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

// Get читает значение по ключу и десериализует в dst.
// Возвращает ErrCacheMiss, если ключа нет, ErrCacheUnavailable при проблеме с соединением.
func (s *Service) Get(ctx context.Context, key string, dst interface{}) error {
	if !s.IsEnabled() {
		return ErrCacheUnavailable
	}
	raw, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrCacheMiss
		}
		// Любая иная ошибка — соединение, таймаут и т.д.
		log.Printf("[cache] GET %s: %v", key, err)
		return ErrCacheUnavailable
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		// Битый JSON в кеше — лучше удалить, чем хранить.
		log.Printf("[cache] невалидный JSON для %s: %v, удаляю ключ", key, err)
		_ = s.client.Del(ctx, key).Err()
		return ErrCacheMiss
	}
	return nil
}

// GetString читает чистую строку (без JSON-парсинга). Удобно для JTI access токенов:
// там значение — это "valid" или userID, ради простоты не сериализуем как JSON.
func (s *Service) GetString(ctx context.Context, key string) (string, error) {
	if !s.IsEnabled() {
		return "", ErrCacheUnavailable
	}
	v, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrCacheMiss
		}
		log.Printf("[cache] GET %s: %v", key, err)
		return "", ErrCacheUnavailable
	}
	return v, nil
}

// Set сохраняет value, сериализуя через JSON, с заданным TTL.
// Если ttl == 0 — используется defaultTTL.
// TTL обязателен по требованиям лабы — нулевой / отрицательный TTL не допускается.
func (s *Service) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if !s.IsEnabled() {
		return ErrCacheUnavailable
	}
	if ttl <= 0 {
		ttl = s.defaultTTL
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("сериализация в JSON: %w", err)
	}
	if err := s.client.Set(ctx, key, data, ttl).Err(); err != nil {
		log.Printf("[cache] SET %s: %v", key, err)
		return ErrCacheUnavailable
	}
	return nil
}

// SetString — то же, что Set, но без JSON-обёртки. Для JTI и подобных коротких маркеров.
func (s *Service) SetString(ctx context.Context, key, value string, ttl time.Duration) error {
	if !s.IsEnabled() {
		return ErrCacheUnavailable
	}
	if ttl <= 0 {
		ttl = s.defaultTTL
	}
	if err := s.client.Set(ctx, key, value, ttl).Err(); err != nil {
		log.Printf("[cache] SET %s: %v", key, err)
		return ErrCacheUnavailable
	}
	return nil
}

// Exists проверяет существование ключа (для guard-проверки JTI).
func (s *Service) Exists(ctx context.Context, key string) (bool, error) {
	if !s.IsEnabled() {
		return false, ErrCacheUnavailable
	}
	n, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		log.Printf("[cache] EXISTS %s: %v", key, err)
		return false, ErrCacheUnavailable
	}
	return n > 0, nil
}

// Del удаляет один или несколько ключей.
func (s *Service) Del(ctx context.Context, keys ...string) error {
	if !s.IsEnabled() || len(keys) == 0 {
		return nil
	}
	if err := s.client.Del(ctx, keys...).Err(); err != nil {
		log.Printf("[cache] DEL %v: %v", keys, err)
		return ErrCacheUnavailable
	}
	return nil
}

// DelByPattern удаляет все ключи, подходящие под паттерн (например "wp:items:list:*").
// Реализовано через SCAN + UNLINK — это безопаснее KEYS+DEL на проде:
//   - SCAN не блокирует Redis на больших датасетах
//   - UNLINK освобождает память асинхронно
//
// Один из контрольных вопросов лабы как раз про KEYS — здесь сознательно НЕ используем его.
func (s *Service) DelByPattern(ctx context.Context, pattern string) error {
	if !s.IsEnabled() {
		return nil
	}
	if !strings.HasPrefix(pattern, AppPrefix+":") {
		// Подстраховка: запрещаем массовое удаление без префикса приложения.
		return fmt.Errorf("паттерн %q должен начинаться с %q:", pattern, AppPrefix)
	}

	var cursor uint64
	deleted := 0
	for {
		keys, next, err := s.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			log.Printf("[cache] SCAN %s: %v", pattern, err)
			return ErrCacheUnavailable
		}
		if len(keys) > 0 {
			if err := s.client.Unlink(ctx, keys...).Err(); err != nil {
				log.Printf("[cache] UNLINK %v: %v", keys, err)
			} else {
				deleted += len(keys)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	if deleted > 0 {
		log.Printf("[cache] DelByPattern %s: удалено %d ключей", pattern, deleted)
	}
	return nil
}

// --- Helpers для построения ключей ---
// Все ключи приложения должны конструироваться через эти функции,
// чтобы префиксная схема жила в одном месте.
//
// Лаба 6: userID и itemID теперь string (hex от MongoDB ObjectID), а не uint.
// Шаблон тот же: wp:модуль:сущность:идентификатор.

// KeyItemsList — кеш страницы списка items конкретного пользователя.
// Пагинация (page/limit) включена в ключ — разные страницы лежат раздельно.
func KeyItemsList(userID string, page, limit int) string {
	return fmt.Sprintf("%s:items:user:%s:list:page:%d:limit:%d", AppPrefix, userID, page, limit)
}

// KeyItemsListPatternForUser — паттерн для массовой инвалидации списков юзера
// (при POST/PUT/DELETE мы не знаем, какие конкретно страницы закешированы).
func KeyItemsListPatternForUser(userID string) string {
	return fmt.Sprintf("%s:items:user:%s:list:*", AppPrefix, userID)
}

// KeyItemByID — кеш одного item.
func KeyItemByID(userID, itemID string) string {
	return fmt.Sprintf("%s:items:user:%s:item:%s", AppPrefix, userID, itemID)
}

// KeyUserProfile — кеш профиля для /auth/whoami.
func KeyUserProfile(userID string) string {
	return fmt.Sprintf("%s:users:profile:%s", AppPrefix, userID)
}

// KeyAccessJTI — реестр валидных access-токенов конкретного пользователя.
// Используется для мгновенного отзыва access токенов в /logout (см. задание, п.4).
func KeyAccessJTI(userID, jti string) string {
	return fmt.Sprintf("%s:auth:user:%s:access:%s", AppPrefix, userID, jti)
}

// KeyAccessJTIPatternForUser — все access-JTI пользователя (для logout-all).
func KeyAccessJTIPatternForUser(userID string) string {
	return fmt.Sprintf("%s:auth:user:%s:access:*", AppPrefix, userID)
}
