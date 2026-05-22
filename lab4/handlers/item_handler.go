package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"awesomeProject/dto"
	"awesomeProject/middleware"
	"awesomeProject/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ItemHandler — CRUD ресурсов, привязанных к текущему пользователю.
type ItemHandler struct {
	Service *services.ItemService
}

// handleItemError — единая обработка ошибок item-операций.
func handleItemError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
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

	item, err := h.Service.Create(userID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Не удалось создать элемент"})
		return
	}
	c.JSON(http.StatusCreated, item)
}

// GetAll godoc
// @Summary      Получить список ресурсов с пагинацией
// @Description  Возвращает только items текущего пользователя. Параметры page и limit — query-string.
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

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	response, err := h.Service.GetAll(userID, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Ошибка сервера"})
		return
	}
	c.JSON(http.StatusOK, response)
}

// GetByID godoc
// @Summary      Получить ресурс по ID
// @Tags         Items
// @Produce      json
// @Param        id   path      int  true  "ID ресурса"
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
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверный ID"})
		return
	}
	item, err := h.Service.GetByID(userID, uint(id))
	if err != nil {
		handleItemError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// Update godoc
// @Summary      Полное обновление ресурса (PUT)
// @Description  Полностью заменяет name и description. Доступно только владельцу ресурса.
// @Tags         Items
// @Accept       json
// @Produce      json
// @Param        id    path      int                    true  "ID ресурса"
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
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверный ID"})
		return
	}
	var req dto.ItemCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверные данные: " + err.Error()})
		return
	}
	item, err := h.Service.Update(userID, uint(id), req)
	if err != nil {
		handleItemError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// Patch godoc
// @Summary      Частичное обновление ресурса (PATCH)
// @Description  Обновляет только переданные поля. Доступно только владельцу.
// @Tags         Items
// @Accept       json
// @Produce      json
// @Param        id    path      int                  true  "ID ресурса"
// @Param        body  body      dto.ItemPatchRequest true  "Поля для частичного обновления"
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
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверный ID"})
		return
	}
	var req dto.ItemPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверные данные: " + err.Error()})
		return
	}
	item, err := h.Service.Patch(userID, uint(id), req)
	if err != nil {
		handleItemError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// Delete godoc
// @Summary      Удалить ресурс (soft delete)
// @Description  Помечает запись удалённой (gorm.DeletedAt), физически данные сохраняются. Доступно только владельцу.
// @Tags         Items
// @Param        id   path  int  true  "ID ресурса"
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
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Неверный ID"})
		return
	}
	if err := h.Service.Delete(userID, uint(id)); err != nil {
		handleItemError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
