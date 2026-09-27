package routes

import (
	"dormcheck/database"
	"dormcheck/middleware"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
)

type updateInput struct {
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	EventDate string `json:"event_date"`
	Pinned    bool   `json:"pinned"`
	Published bool   `json:"published"`
}

// publicUpdate deliberately omits author identifiers from the public feed.
type publicUpdate struct {
	ID          uint       `json:"id"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	EventDate   time.Time  `json:"event_date"`
	Pinned      bool       `json:"pinned"`
	Published   bool       `json:"published"`
	PublishedAt *time.Time `json:"published_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func RegisterUpdateRoutes(app *fiber.App) {
	app.Get("/updates", func(c *fiber.Ctx) error { return listUpdates(c, true) })
	admin := app.Group("/admin/updates", middleware.JwtAuth, middleware.AdminAuth)
	admin.Get("/", func(c *fiber.Ctx) error { return listUpdates(c, false) })
	admin.Post("/", createUpdate)
	admin.Put("/:id", editUpdate)
	admin.Delete("/:id", deleteUpdate)
}

func listUpdates(c *fiber.Ctx, public bool) error {
	page, size := c.QueryInt("page", 1), c.QueryInt("page_size", 10)
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 50 {
		size = 50
	}
	query := database.DB.Model(&database.PlatformPost{})
	if public {
		query = query.Where("published = ?", true)
	}
	if kind := c.Query("kind"); kind == "update" || kind == "announcement" {
		query = query.Where("kind = ?", kind)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "读取更新记录失败"})
	}
	posts := make([]database.PlatformPost, 0)
	if err := query.Order("pinned DESC").Order("event_date DESC").Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&posts).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "读取更新记录失败"})
	}
	if public {
		publicPosts := make([]publicUpdate, 0, len(posts))
		for _, post := range posts {
			publicPosts = append(publicPosts, publicUpdate{
				ID: post.ID, Kind: post.Kind, Title: post.Title, Body: post.Body,
				EventDate: post.EventDate, Pinned: post.Pinned, Published: post.Published,
				PublishedAt: post.PublishedAt, CreatedAt: post.CreatedAt, UpdatedAt: post.UpdatedAt,
			})
		}
		return c.JSON(fiber.Map{"posts": publicPosts, "total": total, "page": page, "page_size": size})
	}
	return c.JSON(fiber.Map{"posts": posts, "total": total, "page": page, "page_size": size})
}

func parseUpdateInput(c *fiber.Ctx) (updateInput, time.Time, error) {
	var input updateInput
	if err := c.BodyParser(&input); err != nil {
		return input, time.Time{}, err
	}
	input.Kind = strings.TrimSpace(input.Kind)
	input.Title = strings.TrimSpace(input.Title)
	input.Body = strings.TrimSpace(input.Body)
	if input.Kind != "update" && input.Kind != "announcement" {
		return input, time.Time{}, fiber.NewError(400, "请选择记录类型")
	}
	if n := utf8.RuneCountInString(input.Title); n < 1 || n > 120 {
		return input, time.Time{}, fiber.NewError(400, "标题需为 1 至 120 字")
	}
	if n := utf8.RuneCountInString(input.Body); n < 1 || n > 20000 {
		return input, time.Time{}, fiber.NewError(400, "正文需为 1 至 20000 字")
	}
	date, err := time.ParseInLocation("2006-01-02", input.EventDate, time.Local)
	if err != nil {
		return input, time.Time{}, fiber.NewError(400, "请选择有效的记录日期")
	}
	return input, date, nil
}

func inputError(c *fiber.Ctx, err error) error {
	if fiberErr, ok := err.(*fiber.Error); ok {
		return c.Status(fiberErr.Code).JSON(fiber.Map{"error": fiberErr.Message})
	}
	return c.Status(400).JSON(fiber.Map{"error": "请求参数无效"})
}

func applyUpdateInput(post *database.PlatformPost, input updateInput, date time.Time) {
	post.Kind, post.Title, post.Body, post.EventDate = input.Kind, input.Title, input.Body, date
	post.Pinned, post.Published = input.Pinned, input.Published
	if !input.Published {
		post.PublishedAt = nil
	} else if post.PublishedAt == nil {
		now := time.Now()
		post.PublishedAt = &now
	}
}

func createUpdate(c *fiber.Ctx) error {
	input, date, err := parseUpdateInput(c)
	if err != nil {
		return inputError(c, err)
	}
	actor, ok := c.Locals("user").(*database.User)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "登录状态无效"})
	}
	post := database.PlatformPost{AuthorID: actor.ID, AuthorName: actor.Username}
	applyUpdateInput(&post, input, date)
	if err := database.DB.Create(&post).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "保存更新记录失败"})
	}
	return c.Status(201).JSON(post)
}

func updateID(c *fiber.Ctx) (uint, error) {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || id == 0 {
		return 0, fiber.NewError(400, "记录 ID 无效")
	}
	return uint(id), nil
}

func editUpdate(c *fiber.Ctx) error {
	id, err := updateID(c)
	if err != nil {
		return inputError(c, err)
	}
	input, date, err := parseUpdateInput(c)
	if err != nil {
		return inputError(c, err)
	}
	var post database.PlatformPost
	if err := database.DB.First(&post, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "记录不存在"})
	}
	applyUpdateInput(&post, input, date)
	if err := database.DB.Save(&post).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "保存更新记录失败"})
	}
	return c.JSON(post)
}

func deleteUpdate(c *fiber.Ctx) error {
	id, err := updateID(c)
	if err != nil {
		return inputError(c, err)
	}
	result := database.DB.Delete(&database.PlatformPost{}, id)
	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": "删除更新记录失败"})
	}
	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "记录不存在"})
	}
	return c.JSON(fiber.Map{"message": "记录已删除"})
}
