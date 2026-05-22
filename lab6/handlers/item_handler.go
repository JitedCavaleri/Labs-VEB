package handlers

import (
	"errors"
	"net/http"

	"awesomeProject/database"
	"awesomeProject/dto"
	"awesomeProject/middleware"
	"awesomeProject/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ItemHandler — CRUD ресурсов, привязанных к текущему пользователю.
//
// Лаба 6: id в URL — это hex от MongoDB ObjectID (24 hex символа), а не uint.
// Парсинг отдан primitive.ObjectIDFromHex, который сразу же отбрасывает явно битые ID
// с 400 Bad Request.
type ItemHandler struct {
	Service *services.ItemService
}

// parseObjectID — валидируем :id из URL.
// Возвращает hex-строку (для передачи в сервис) и флаг успеха.
// При неудаче отвечает 400 Bad Request клиенту.
func parseObjectID(c *gin.Context) (string, bool) {
	raw := c.Param("id")
	if _, err := primitive.ObjectIDFromHex(raw); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверный ID"})
		return "", false
	}
	return raw, true
}

// handleItemError — единая обработка ошибок item-операций.
func handleItemError(c *gin.Context, err error) {
	switch {
	case database.IsNotFound(err):
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Элемент не найден"})
	case errors.Is(err, services.ErrForbidden):
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "Недостаточно прав"})
	default:
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Внутренняя ошибка сервера"})
	}
}

// Create godoc
// @Summary      Создать новый ресурс
// @Description  Создаёт item, владельцем становится текущий пользователь. Чужие пользователи не имеют к нему доступа.
// @Description  Лаба 5: после успешного создания инвалидируется кеш всех страниц списка пользователя.
// @Description  Лаба 6: документ сохраняется в коллекцию items, _id — ObjectID.
// @Tags         Items
// @Accept       json
// @Produce      json
// @Param        body  body      dto.ItemCreateRequest  true  "Данные нового ресурса"
// @Success      201   {object}  models.Item
// @Failure      400   {object}  dto.ErrorResponse "Невалидные данные запроса"
// @Failure      401   {object}  dto.ErrorResponse "Не авторизован"
// @Failure      500   {object}  dto.ErrorResponse
// @Security     CookieAuth
// @Security     BearerAuth
// @Router       /items [post]
func (h *ItemHandler) Create(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Не авторизован"})
		return
	}

	var req dto.ItemCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверные данные: " + err.Error()})
		return
	}

	item, err := h.Service.Create(c.Request.Context(), userID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Не удалось создать элемент"})
		return
	}
	c.JSON(http.StatusCreated, item)
}

// GetAll godoc
// @Summary      Получить список ресурсов с пагинацией
// @Description  Возвращает только items текущего пользователя. Параметры page и limit — query-string.
// @Description  Лаба 5: ответ кешируется в Redis под ключом wp:items:user:{id}:list:page:{p}:limit:{l} с TTL.
// @Description  Лаба 6: пагинация на MongoDB через skip+limit, фильтр {"userId":..., "deletedAt": null}.
// @Tags         Items
// @Produce      json
// @Param        page   query     int  false  "Номер страницы (>=1)"  default(1)
// @Param        limit  query     int  false  "Размер страницы (1..100)"  default(10)
// @Success      200    {object}  dto.PaginationResponse
// @Failure      401    {object}  dto.ErrorResponse "Не авторизован"
// @Failure      500    {object}  dto.ErrorResponse
// @Security     CookieAuth
// @Security     BearerAuth
// @Router       /items [get]
func (h *ItemHandler) GetAll(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Не авторизован"})
		return
	}

	page := parseIntDefault(c.DefaultQuery("page", "1"), 1)
	limit := parseIntDefault(c.DefaultQuery("limit", "10"), 10)

	response, err := h.Service.GetAll(c.Request.Context(), userID, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Ошибка сервера"})
		return
	}
	c.JSON(http.StatusOK, response)
}

// GetByID godoc
// @Summary      Получить ресурс по ID
// @Description  Лаба 5: значение кешируется в Redis под ключом wp:items:user:{id}:item:{itemId}.
// @Description  Лаба 6: id в URL — hex от MongoDB ObjectID (24 hex символа).
// @Tags         Items
// @Produce      json
// @Param        id   path      string  true  "ID ресурса (24 hex символа)"  example(6555c0a3d8e94f1b3c5a0003)
// @Success      200  {object}  models.Item
// @Failure      400  {object}  dto.ErrorResponse "Неверный ID"
// @Failure      401  {object}  dto.ErrorResponse "Не авторизован"
// @Failure      403  {object}  dto.ErrorResponse "Ресурс принадлежит другому пользователю"
// @Failure      404  {object}  dto.ErrorResponse "Элемент не найден"
// @Security     CookieAuth
// @Security     BearerAuth
// @Router       /items/{id} [get]
func (h *ItemHandler) GetByID(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Не авторизован"})
		return
	}
	id, ok := parseObjectID(c)
	if !ok {
		return
	}
	item, err := h.Service.GetByID(c.Request.Context(), userID, id)
	if err != nil {
		handleItemError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// Update godoc
// @Summary      Полное обновление ресурса (PUT)
// @Description  Полностью заменяет name и description. Доступно только владельцу ресурса.
// @Description  Лаба 5: инвалидирует кеш списка и кеш конкретного элемента.
// @Tags         Items
// @Accept       json
// @Produce      json
// @Param        id    path      string                 true  "ID ресурса (24 hex символа)"
// @Param        body  body      dto.ItemCreateRequest  true  "Новые данные"
// @Success      200   {object}  models.Item
// @Failure      400   {object}  dto.ErrorResponse "Неверные данные или ID"
// @Failure      401   {object}  dto.ErrorResponse "Не авторизован"
// @Failure      403   {object}  dto.ErrorResponse "Чужой ресурс"
// @Failure      404   {object}  dto.ErrorResponse "Элемент не найден"
// @Security     CookieAuth
// @Security     BearerAuth
// @Router       /items/{id} [put]
func (h *ItemHandler) Update(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Не авторизован"})
		return
	}
	id, ok := parseObjectID(c)
	if !ok {
		return
	}
	var req dto.ItemCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверные данные: " + err.Error()})
		return
	}
	item, err := h.Service.Update(c.Request.Context(), userID, id, req)
	if err != nil {
		handleItemError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// Patch godoc
// @Summary      Частичное обновление ресурса (PATCH)
// @Description  Обновляет только переданные поля. Доступно только владельцу.
// @Description  Лаба 5: инвалидирует кеш списка и кеш конкретного элемента.
// @Tags         Items
// @Accept       json
// @Produce      json
// @Param        id    path      string                true  "ID ресурса (24 hex символа)"
// @Param        body  body      dto.ItemPatchRequest  true  "Поля для частичного обновления"
// @Success      200   {object}  models.Item
// @Failure      400   {object}  dto.ErrorResponse "Неверные данные или ID"
// @Failure      401   {object}  dto.ErrorResponse "Не авторизован"
// @Failure      403   {object}  dto.ErrorResponse "Чужой ресурс"
// @Failure      404   {object}  dto.ErrorResponse "Элемент не найден"
// @Security     CookieAuth
// @Security     BearerAuth
// @Router       /items/{id} [patch]
func (h *ItemHandler) Patch(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Не авторизован"})
		return
	}
	id, ok := parseObjectID(c)
	if !ok {
		return
	}
	var req dto.ItemPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверные данные: " + err.Error()})
		return
	}
	item, err := h.Service.Patch(c.Request.Context(), userID, id, req)
	if err != nil {
		handleItemError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// Delete godoc
// @Summary      Удалить ресурс (soft delete)
// @Description  Помечает запись удалённой (поле deletedAt), физически документ сохраняется в коллекции.
// @Description  Все читающие запросы (GET, GetByID, список) автоматически фильтруют по deletedAt == null.
// @Description  Доступно только владельцу. Лаба 5: инвалидирует кеш списка и кеш конкретного элемента.
// @Tags         Items
// @Param        id   path  string  true  "ID ресурса (24 hex символа)"
// @Success      204  "No Content"
// @Failure      400  {object}  dto.ErrorResponse "Неверный ID"
// @Failure      401  {object}  dto.ErrorResponse "Не авторизован"
// @Failure      403  {object}  dto.ErrorResponse "Чужой ресурс"
// @Failure      404  {object}  dto.ErrorResponse "Элемент не найден"
// @Security     CookieAuth
// @Security     BearerAuth
// @Router       /items/{id} [delete]
func (h *ItemHandler) Delete(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "Не авторизован"})
		return
	}
	id, ok := parseObjectID(c)
	if !ok {
		return
	}
	if err := h.Service.Delete(c.Request.Context(), userID, id); err != nil {
		handleItemError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// parseIntDefault — мини-хелпер для безопасного парсинга query-string чисел.
func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
		if n > 1_000_000 {
			return def
		}
	}
	return n
}
