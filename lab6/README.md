# Лабораторная работа №6 — MongoDB

Миграция приложения из Лабораторной работы №5 c PostgreSQL/GORM на MongoDB.
Все остальные механизмы (JWT в HttpOnly cookies, OAuth 2.0 Yandex/VK, Redis-кеш,
Swagger UI, Soft Delete, пагинация, проверка владения ресурсами) сохранены.

## Стек

- **Go 1.23** + **Gin** — HTTP-сервер.
- **MongoDB 6** + официальный драйвер `go.mongodb.org/mongo-driver` — документная БД.
- **Redis 7** — кеширование списков/профилей и реестр валидных access-JTI.
- **JWT (HS256)** в HttpOnly cookies — аутентификация.
- **OAuth 2.0** Yandex/VK — реализация Authorization Code Grant вручную.
- **swaggo/swag** — авто-генерация OpenAPI из аннотаций в коде.
- **mongo-express** — веб-UI для инспекции коллекций (заменяет pgAdmin из лабы 5).

## Быстрый старт

```bash
# 1. Подготовить переменные окружения
cp .env.example .env
# отредактируйте .env при необходимости — дефолтные значения рабочие "из коробки"

# 2. Поднять всё одной командой
docker-compose up --build

# 3. Проверить, что приложение слушает
curl http://localhost:4200/info
```

После старта доступны:
- API: `http://localhost:4200`
- Swagger UI: `http://localhost:4200/api/docs` (только если `APP_ENV != production`)
- mongo-express: `http://localhost:8081` (логин/пароль `admin`/`admin`)

## Архитектура

```
.
├── main.go               # точка входа, маршрутизация Gin
├── config/               # загрузка .env -> Config
├── database/             # подключение к MongoDB, создание индексов, soft-delete helpers
├── cache/                # Redis-обёртка с префиксной схемой ключей
├── models/               # доменные модели (User, Token, Item, PasswordResetToken)
├── dto/                  # request/response контракты для Gin (binding + Swagger)
├── handlers/             # HTTP-слой: парсинг параметров, маппинг ошибок -> коды
├── middleware/           # AuthMiddleware (JWT + JTI guard)
├── services/             # бизнес-логика: auth, items, oauth, password_reset
├── utils/                # JWT, password hashing, cookies
├── docs/                 # генерируется swag init (placeholder в репо)
├── Dockerfile
├── docker-compose.yml
├── .env.example
└── README.md
```

### Что изменилось по сравнению с лабой 5

| Аспект | Лаба 5 (PostgreSQL) | Лаба 6 (MongoDB) |
|---|---|---|
| СУБД | PostgreSQL 16 | MongoDB 6 |
| Доступ к данным | GORM (`gorm.io/gorm`) | mongo-driver (`go.mongodb.org/mongo-driver`) |
| Идентификаторы | `uint`, автоинкремент | `primitive.ObjectID` (24 hex символа) |
| Связи | Foreign keys + CASCADE | Ссылки (References): `userId` в `tokens`/`items` |
| Миграции | `db.AutoMigrate(&Model{})` | Создание индексов при старте (`database.Migrate`) |
| Soft Delete | `gorm.DeletedAt` (автомат) | Поле `deletedAt *time.Time`, фильтр `MergeAlive` |
| Уникальность email | `uniqueIndex` (всегда) | Partial-index по `deletedAt == null` |
| Транзакции | `db.Transaction(func(tx) {…})` | Не используются (single-instance Mongo) |
| Пагинация | `Offset + Limit` | `skip + limit` + sort по `_id desc` |
| Веб-клиент БД | pgAdmin (`:5050`) | mongo-express (`:8081`) |

### Схема данных в MongoDB

База: `wp_labs` (имя из `DB_NAME` / path-части `MONGO_URI`).

| Коллекция | Назначение | Ключевые поля |
|---|---|---|
| `users` | пользователи | `_id`, `email` (uniq alive), `passwordHash`, `salt`, `yandexId`, `vkId`, `deletedAt` |
| `tokens` | refresh-токены (hash + JTI) | `_id`, `userId`, `tokenHash`, `jti` (uniq), `expiresAt`, `revoked`, `deletedAt` |
| `password_reset_tokens` | одноразовые токены восстановления | `_id`, `userId`, `tokenHash`, `expiresAt`, `used` |
| `items` | пользовательские ресурсы (CRUD) | `_id`, `userId`, `name`, `description`, `deletedAt` |

Индексы создаются автоматически при старте приложения (см. `database/Migrate`).

### Почему ссылки, а не embedding

`tokens` и `items` — отдельные коллекции, а не массивы внутри документа User, потому что:

1. **Items могут расти неограниченно** — пагинация в API требует, чтобы они были запрашиваемой выборкой, а не подмассивом одного документа (упрёмся в лимит 16MB на документ).
2. **`/items/:id` ищет ресурс глобально** по `_id`, потом проверяет владение. Embedding потребовал бы знать `userId` заранее — это ломает API-контракт.
3. **Refresh-токены ротируются** на каждом `/auth/refresh`. Embedding потребовал бы переписывать User-документ целиком на каждый запрос — это лишний I/O.

### Soft Delete

Поле `deletedAt *time.Time`:
- `nil` (отсутствует) — запись жива.
- Установлено в время удаления — запись soft-deleted.

Все читающие запросы оборачиваются через `database.MergeAlive(filter)` — он добавляет в фильтр `{"deletedAt": nil}`. Реализация — на уровне приложения, потому что mongo-driver, в отличие от GORM, не умеет автоматический Soft Delete.

Уникальный индекс по `email` сделан partial (`partialFilterExpression: deletedAt == null`), чтобы после soft-delete тот же email можно было использовать повторно.

## API

### Public

| Метод | Путь | Описание |
|---|---|---|
| GET | `/info` | Количество дней до Нового года (из лабы 2) |
| GET | `/api/docs` | Swagger UI (только не в production) |

### Auth

| Метод | Путь | Описание |
|---|---|---|
| POST | `/auth/register` | Регистрация. Возвращает HttpOnly cookies access/refresh |
| POST | `/auth/login` | Вход. Возвращает HttpOnly cookies access/refresh |
| POST | `/auth/refresh` | Обновление токенов (rotation). Старый refresh отзывается |
| POST | `/auth/forgot-password` | Запрос ссылки сброса пароля. Всегда 200 (защита от user enumeration) |
| POST | `/auth/reset-password` | Установка нового пароля по reset-токену. Отзывает все сессии |
| GET | `/auth/whoami` | 🔒 Профиль текущего пользователя (кеш Redis) |
| POST | `/auth/logout` | 🔒 Logout текущей сессии. Удаляет access-JTI из Redis |
| POST | `/auth/logout-all` | 🔒 Logout со всех устройств. Удаляет все access-JTI и сбрасывает кеш |

### OAuth

| Метод | Путь | Описание |
|---|---|---|
| GET | `/auth/oauth/yandex` | 302 -> страница согласия Yandex |
| GET | `/auth/oauth/yandex/callback` | Принимает code/state, обменивает на токены |
| GET | `/auth/oauth/vk` | 302 -> страница согласия VK |
| GET | `/auth/oauth/vk/callback` | Принимает code/state, обменивает на токены |

### Items (CRUD, защищено)

| Метод | Путь | Описание |
|---|---|---|
| POST | `/items` | 🔒 Создать ресурс |
| GET | `/items?page=1&limit=10` | 🔒 Список (пагинация, фильтр по владельцу, кеш Redis) |
| GET | `/items/{id}` | 🔒 Один ресурс по hex-`_id` (24 символа) |
| PUT | `/items/{id}` | 🔒 Полное обновление |
| PATCH | `/items/{id}` | 🔒 Частичное обновление |
| DELETE | `/items/{id}` | 🔒 Soft Delete (`deletedAt` ставится в now) |

🔒 — требует `access_token` cookie (или `Authorization: Bearer <token>` для отладки в Swagger).

## Примеры запросов (curl)

```bash
# Регистрация (cookies сохраняем в jar)
curl -i -c cookies.txt -X POST http://localhost:4200/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"test@example.com","password":"StrongPass123","phone":"+79991234567"}'

# Whoami через cookie
curl -b cookies.txt http://localhost:4200/auth/whoami

# Создать item
curl -b cookies.txt -X POST http://localhost:4200/items \
  -H 'Content-Type: application/json' \
  -d '{"name":"Молоток","description":"Стальной молоток с деревянной рукояткой"}'

# Список с пагинацией
curl -b cookies.txt 'http://localhost:4200/items?page=1&limit=5'

# Soft delete
curl -b cookies.txt -X DELETE http://localhost:4200/items/<ID>

# Убедиться, что удалённый ресурс больше не возвращается
curl -b cookies.txt http://localhost:4200/items/<ID>   # 404
curl -b cookies.txt 'http://localhost:4200/items'      # не содержит <ID>

# Logout
curl -b cookies.txt -X POST http://localhost:4200/auth/logout
```

## Проверка сохранения данных в MongoDB

```bash
# Подключиться к контейнеру
docker exec -it wp_labs_mongo mongosh -u student -p student_secure_password --authenticationDatabase admin

# В mongosh:
use wp_labs
show collections                       # users tokens items password_reset_tokens
db.users.find().pretty()
db.items.find({ deletedAt: null }).pretty()         # "живые" ресурсы
db.items.find({ deletedAt: { $ne: null } }).count() # сколько soft-deleted
db.users.getIndexes()                  # должно быть uniq_email_alive (partial)
```

Альтернатива — открыть `http://localhost:8081` (mongo-express, логин/пароль `admin`/`admin`).

## Что соответствует критериям приёмки

- ✅ **Репозиторий** — Go-проект с модульной структурой (handlers / services / models / dto / middleware / utils / cache / database / config).
- ✅ **README** — этот файл: описание, запуск через `docker-compose up --build`, `.env.example`, список эндпоинтов.
- ✅ **Все HTTP-методы** работают (GET, POST, PUT, PATCH, DELETE).
- ✅ **Soft Delete** — поле `deletedAt`, фильтр `MergeAlive` во всех читающих запросах.
- ✅ **Пагинация** — `?page=...&limit=...`, `skip + limit` в Mongo, `totalPages` в ответе.
- ✅ **Авторизация и кеш** — JWT в cookies, access-JTI в Redis для мгновенного отзыва, кеш списков/профиля.
- ✅ **Запросы не возвращают удалённые** — каждый Find/FindOne идёт через `MergeAlive`, удалённые исключаются автоматически.
- ✅ **Модульная структура** — каждый компонент в отдельном пакете, явное dependency injection в `main.go`.
- ✅ **Валидация данных** — Gin binding-теги в DTO + проверка силы пароля в `AuthService`.
- ✅ **Переменные окружения** — все секреты в `.env`, в коде через `os.Getenv` / `godotenv`.
- ✅ **Чистая БД** — индексы создаются при старте, схема создаётся неявно при первой записи.
- ✅ **MongoDB защищён паролем** — root-пользователь и пароль из `DB_USER`/`DB_PASSWORD`, `authSource=admin`.

## Контрольные вопросы (краткие ответы)

1. **Документная vs реляционная БД?** Реляционная хранит данные в таблицах с фиксированной схемой и связями через FK. Документная (MongoDB) хранит произвольные JSON-подобные документы (BSON) с гибкой схемой; связи делаются ссылками или встраиванием.
2. **BSON vs JSON?** BSON (Binary JSON) — бинарный формат хранения с поддержкой типов, которых нет в JSON: ObjectID, Date, Decimal128, Binary, регулярные выражения. Эффективнее парсится и занимает меньше места.
3. **Embedding vs References?** Embedding выгоден при тесной связи 1:1 / 1-к-малому, когда данные читаются всегда вместе (один read, нет JOIN). References — при 1-ко-многим/многим-ко-многим, когда поддокументы растут, нужно отдельно их запрашивать или они шарятся между владельцами.
4. **Целостность в MongoDB?** Транзакции есть, но только на replica set / sharded cluster. На уровне приложения — валидация через ODM и/или JSON-Schema validator в самой коллекции. Уникальность — через индексы.
5. **Запись неверного типа в поле схемы ODM?** ODM (Mongoose / Beanie / Spring Data) вернёт ошибку валидации до отправки в БД. В нативном драйвере Go типы проверяются на этапе компиляции и `bson.Marshal`; MongoDB сам по себе примет любой тип, если на коллекции нет JSON-Schema validator'a.
6. **Влияние отсутствия JOIN-ов?** Денормализация — встраивай данные, которые читаются вместе. Альтернатива — `$lookup` в aggregation pipeline (медленнее SQL JOIN). Часть бизнес-логики переезжает из БД в приложение.
7. **Зачем индексы и их цена?** Без индекса — full scan коллекции. С индексом — B-tree поиск за O(log n). Цена: каждая запись пересчитывает индексы → INSERT/UPDATE медленнее, занимает место на диске и в RAM.
8. **Уникальность email?** Уникальный индекс: `db.users.createIndex({email:1}, {unique:true})`. В этом проекте — partial-index по `deletedAt == null`, чтобы soft-deleted email можно было переиспользовать.
9. **Сценарии MongoDB vs PostgreSQL?** Mongo — гибкая/эволюционирующая схема, JSON-heavy данные, горизонтальное масштабирование, content management, аналитика логов/событий. PostgreSQL — строгие транзакции, сложные JOIN-ы, финансы/учёт, отчётность, когда схема стабильна и реляционные связи естественны.
