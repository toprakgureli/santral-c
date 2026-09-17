package escalation

import (
	"io"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/validator"
)

// maxImportBytes caps the size of an uploaded catalog spreadsheet.
const maxImportBytes = 5 << 20

// Handler serves the escalation endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds an escalation handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Categories lists the escalation catalog.
func (h *Handler) Categories(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Categories(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// CreateCategory adds a category.
func (h *Handler) CreateCategory(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.EscalationCategoryCreate
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.CreateCategory(c.UserContext(), id, req)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// orderBody is the id list a reorder request carries, first to last.
type orderBody struct {
	IDs []uint `json:"ids"`
}

// ReorderCategories saves the category order.
func (h *Handler) ReorderCategories(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req orderBody
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.ReorderCategories(c.UserContext(), id, req.IDs); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ReorderReasons saves the reason order inside a category.
func (h *Handler) ReorderReasons(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	catID, err := param(c, "id")
	if err != nil {
		return err
	}
	var req orderBody
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.ReorderReasons(c.UserContext(), id, catID, req.IDs); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// DeleteCategory removes a category.
func (h *Handler) DeleteCategory(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	target, err := param(c, "id")
	if err != nil {
		return err
	}
	if err := h.service.DeleteCategory(c.UserContext(), id, target); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// CreateReason adds a reason under a category.
func (h *Handler) CreateReason(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	catID, err := param(c, "id")
	if err != nil {
		return err
	}
	var req requests.EscalationReasonCreate
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.CreateReason(c.UserContext(), id, catID, req)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// DeleteReason removes a reason.
func (h *Handler) DeleteReason(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	target, err := param(c, "id")
	if err != nil {
		return err
	}
	if err := h.service.DeleteReason(c.UserContext(), id, target); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Import ingests a catalog spreadsheet.
func (h *Handler) Import(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	file, err := c.FormFile("file")
	if err != nil {
		return errs.Invalid("Dosya bulunamadı.", err)
	}
	if file.Size > maxImportBytes {
		return errs.Invalid("Dosya çok büyük (en fazla 5 MB).", nil)
	}
	f, err := file.Open()
	if err != nil {
		return errs.Invalid("Dosya açılamadı.", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxImportBytes))
	if err != nil {
		return errs.Invalid("Dosya okunamadı.", err)
	}
	cats, reasons, err := h.service.Import(c.UserContext(), id, file.Filename, data)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"addedCategories": cats, "addedReasons": reasons})
}

// Log records an escalation for a call.
func (h *Handler) Log(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.EscalationCreate
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.Log(c.UserContext(), id, req)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// LogNone records that a call needed no escalation.
func (h *Handler) LogNone(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.EscalationNone
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.LogNone(c.UserContext(), id, req)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// List returns escalations in pages, scoped by the actor's permissions.
func (h *Handler) List(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	q := ListQuery{
		Number:     c.Query("number"),
		From:       c.Query("from"),
		To:         c.Query("to"),
		Page:       c.QueryInt("page", 1),
		PerPage:    c.QueryInt("perPage", 25),
		AgentID:    uint(c.QueryInt("agentId", 0)),
		CategoryID: uint(c.QueryInt("categoryId", 0)),
	}
	res, err := h.service.List(c.UserContext(), id, q)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Agents lists the agents behind the records (escalation.list_all).
func (h *Handler) Agents(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Agents(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// History returns past escalations for a number.
func (h *Handler) History(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	number := c.Query("number")
	if number == "" {
		return errs.Invalid("number parametresi zorunlu.", nil)
	}
	res, err := h.service.History(c.UserContext(), id, number)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	return id, nil
}

func param(c *fiber.Ctx, name string) (uint, error) {
	id, err := strconv.ParseUint(c.Params(name), 10, 64)
	if err != nil {
		return 0, errs.Invalid("Geçersiz kimlik.", err)
	}
	return uint(id), nil
}
