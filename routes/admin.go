package routes

import (
	"strconv"
	"strings"

	"dormcheck/database"
	"dormcheck/logic/student"
	"dormcheck/logic/user"
	"dormcheck/middleware"
	"errors"

	"github.com/gofiber/fiber/v2"
)

func RegisterAdminRoutes(app *fiber.App) {
	admin := app.Group("/admin", middleware.JwtAuth, middleware.UserNotBanned, middleware.AdminAuth)
	settingsAdmin := app.Group("/admin/settings", middleware.JwtAuth, middleware.UserNotBanned, middleware.SuperAdminAuth)

	settingsAdmin.Get("/", func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		actorID, ok := c.Locals("userID").(int)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "登录状态无效"})
		}
		settings, err := user.GetAdminSettings(actorID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "读取平台设置失败"})
		}
		return c.JSON(settings)
	})

	settingsAdmin.Put("/", func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		actorID, ok := c.Locals("userID").(int)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "登录状态无效"})
		}
		var body struct {
			Values       map[string]string `json:"values"`
			Secrets      map[string]string `json:"secrets"`
			ClearSecrets []string          `json:"clear_secrets"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "请求参数无效"})
		}
		if err := user.UpdateAdminSettings(actorID, body.Values, body.Secrets, body.ClearSecrets); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		settings, err := user.GetAdminSettings(actorID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "设置已保存，但读取回显失败"})
		}
		return c.JSON(fiber.Map{"message": "平台设置已保存", "settings": settings})
	})

	admin.Get("/users", func(c *fiber.Ctx) error {
		page := c.QueryInt("page", 1)
		pageSize := c.QueryInt("page_size", 50)
		if page < 1 {
			page = 1
		}
		if pageSize < 1 {
			pageSize = 50
		}
		if pageSize > 100 {
			pageSize = 100
		}

		result, err := user.ListUsers(c.Query("q"), page, pageSize)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "读取用户列表失败"})
		}
		return c.JSON(result)
	})

	admin.Patch("/users/:id/role", func(c *fiber.Ctx) error {
		targetID, err := strconv.Atoi(c.Params("id"))
		if err != nil || targetID <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "用户 ID 无效"})
		}
		var body struct {
			Role int `json:"role"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "请求参数无效"})
		}
		actorID, ok := c.Locals("userID").(int)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "登录状态无效"})
		}

		updated, err := user.UpdateUserRole(actorID, targetID, body.Role)
		if err != nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "用户角色已更新，用户需重新登录", "user": updated})
	})

	admin.Put("/users/:id/ban", func(c *fiber.Ctx) error {
		targetID, err := strconv.Atoi(c.Params("id"))
		if err != nil || targetID <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "用户 ID 无效"})
		}
		var body struct {
			DurationDays int    `json:"duration_days"`
			Reason       string `json:"reason"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "请求参数无效"})
		}
		actorID, ok := c.Locals("userID").(int)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "登录状态无效"})
		}
		updated, err := user.BanUser(actorID, targetID, body.DurationDays, body.Reason)
		if err != nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "用户已封禁", "user": updated})
	})

	admin.Delete("/users/:id/ban", func(c *fiber.Ctx) error {
		targetID, err := strconv.Atoi(c.Params("id"))
		if err != nil || targetID <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "用户 ID 无效"})
		}
		actorID, ok := c.Locals("userID").(int)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "登录状态无效"})
		}
		if err := user.UnbanUser(actorID, targetID); err != nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "用户封禁已解除"})
	})

	admin.Get("/students", func(c *fiber.Ctx) error {
		page, pageSize := c.QueryInt("page", 1), c.QueryInt("page_size", 25)
		if page < 1 {
			page = 1
		}
		if pageSize < 1 {
			pageSize = 25
		}
		if pageSize > 100 {
			pageSize = 100
		}
		students, total, err := user.ListStudents(c.Query("q"), page, pageSize)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "读取学生账号列表失败"})
		}
		return c.JSON(fiber.Map{"students": students, "total": total, "page": page, "page_size": pageSize})
	})

	admin.Put("/students/:stu_id/credentials", func(c *fiber.Ctx) error {
		stuID := strings.TrimSpace(c.Params("stu_id"))
		var body struct {
			Password string `json:"password"`
		}
		if err := c.BodyParser(&body); err != nil || body.Password == "" || len(body.Password) > 256 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "密码不能为空且不能超过 256 个字符"})
		}
		var existing database.Student
		if err := database.DB.Select("stu_id").First(&existing, "stu_id = ?", stuID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "学生账号不存在"})
		}
		name, err := student.VerifyAndSaveStudentCredentials(stuID, body.Password)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "学生登录信息已验证并更新", "name": name})
	})

	admin.Get("/students/bans", func(c *fiber.Ctx) error {
		page, pageSize := c.QueryInt("page", 1), c.QueryInt("page_size", 50)
		if page < 1 {
			page = 1
		}
		if pageSize < 1 {
			pageSize = 50
		}
		if pageSize > 100 {
			pageSize = 100
		}
		bans, total, err := user.ListStudentBans(c.Query("q"), page, pageSize)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "读取学号封禁列表失败"})
		}
		return c.JSON(fiber.Map{"bans": bans, "total": total, "page": page, "page_size": pageSize})
	})

	admin.Put("/students/:stu_id/ban", func(c *fiber.Ctx) error {
		var body struct {
			DurationDays int    `json:"duration_days"`
			Reason       string `json:"reason"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "请求参数无效"})
		}
		actorID, ok := c.Locals("userID").(int)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "登录状态无效"})
		}
		ban, err := user.BanStudent(actorID, c.Params("stu_id"), body.DurationDays, body.Reason)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "学生账号已封禁，绑定和任务设置已保留，封禁期间不会执行任务", "ban": ban})
	})

	admin.Delete("/students/:stu_id/ban", func(c *fiber.Ctx) error {
		actorID, ok := c.Locals("userID").(int)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "登录状态无效"})
		}
		if err := user.UnbanStudent(actorID, c.Params("stu_id")); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "学生账号封禁已解除，可重新绑定"})
	})

	admin.Get("/tasks", func(c *fiber.Ctx) error {
		activities, err := student.GetActivityTaskSummaries()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "读取活动任务汇总失败"})
		}
		return c.JSON(fiber.Map{"activities": activities})
	})

	admin.Get("/tasks/list", func(c *fiber.Ctx) error {
		page := c.QueryInt("page", 1)
		pageSize := c.QueryInt("page_size", 25)
		if page < 1 {
			page = 1
		}
		if pageSize < 1 {
			pageSize = 25
		}
		if pageSize > 100 {
			pageSize = 100
		}
		tasks, total, err := student.GetAdminTaskList(c.Query("q"), c.Query("activity_id"), c.Query("status"), page, pageSize)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "读取任务列表失败"})
		}
		return c.JSON(fiber.Map{"tasks": tasks, "total": total, "page": page, "page_size": pageSize})
	})

	admin.Get("/tasks/:activity_id", func(c *fiber.Ctx) error {
		tasks, err := student.GetAdminTasksByActivity(c.Params("activity_id"))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "读取活动任务失败"})
		}
		return c.JSON(fiber.Map{"tasks": tasks})
	})

	admin.Put("/tasks/:id", func(c *fiber.Ctx) error {
		taskID, err := strconv.ParseUint(c.Params("id"), 10, 32)
		if err != nil || taskID == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "任务 ID 无效"})
		}
		var body struct {
			Address     string  `json:"address"`
			Longitude   float64 `json:"longitude"`
			Latitude    float64 `json:"latitude"`
			NotifyEmail string  `json:"notify_email"`
		}
		if err := c.BodyParser(&body); err != nil || body.Address == "" || !validCoordinates(body.Longitude, body.Latitude) || !validNotifyEmail(body.NotifyEmail) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "请填写有效的签到地点"})
		}
		body.NotifyEmail = strings.TrimSpace(body.NotifyEmail)
		var task database.Task
		if err := database.DB.First(&task, uint(taskID)).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "任务不存在"})
		}
		result := database.DB.Model(&database.Task{}).Where("id = ? AND running_at IS NULL", task.ID).
			Updates(map[string]interface{}{"address": body.Address, "longitude": body.Longitude, "latitude": body.Latitude, "notify_email": body.NotifyEmail})
		if result.Error != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "保存任务修改失败"})
		}
		if result.RowsAffected == 0 {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "任务正在执行，请稍后修改"})
		}
		_ = database.DB.First(&task, uint(taskID)).Error
		return c.JSON(fiber.Map{"message": "任务修改成功", "task": task})
	})

	admin.Delete("/tasks/:id", func(c *fiber.Ctx) error {
		taskID, err := strconv.ParseUint(c.Params("id"), 10, 32)
		if err != nil || taskID == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "任务 ID 无效"})
		}
		result := database.DB.Where("id = ? AND running_at IS NULL", uint(taskID)).Delete(&database.Task{})
		if result.Error != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "删除任务失败"})
		}
		if result.RowsAffected == 0 {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "任务不存在或正在执行，请稍后重试"})
		}
		return c.JSON(fiber.Map{"message": "任务已删除"})
	})

	admin.Post("/tasks/:id/run", func(c *fiber.Ctx) error {
		taskID64, err := strconv.ParseUint(c.Params("id"), 10, 32)
		if err != nil || taskID64 == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "任务 ID 无效"})
		}
		task, err := student.RunTaskNow(uint(taskID64), c.Locals("userID").(int), true)
		if err != nil {
			if errors.Is(err, student.ErrStudentBanned) {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error(), "message": err.Error()})
			}
			if err == student.ErrManualRunOutsideWindow {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
			if errors.Is(err, student.ErrTaskAlreadyRunning) {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
			}
			if errors.Is(err, student.ErrTaskResultNotSaved) {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "任务不存在或执行状态读取失败"})
		}
		return c.JSON(fiber.Map{"message": "手动签到已执行", "task": task})
	})

	admin.Post("/tasks/:id/toggle", func(c *fiber.Ctx) error {
		taskID64, err := strconv.ParseUint(c.Params("id"), 10, 32)
		if err != nil || taskID64 == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "任务 ID 无效"})
		}
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "请求参数无效"})
		}
		var task database.Task
		if err := database.DB.First(&task, uint(taskID64)).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "任务不存在"})
		}
		if body.Enabled {
			if err := student.EnsureStudentHasEligibleBinding(task.StuID); err != nil {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error(), "message": err.Error()})
			}
			if err := student.EnsureStudentNotBanned(task.StuID); err != nil {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error(), "message": err.Error()})
			}
			if err := student.EnsureStudentCanEnableTasks(task.StuID); err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error(), "message": err.Error()})
			}
			if task.ActivityState != "" && task.ActivityState != "normal" {
				task.ActivityOverride = true
				task.ActivityAutoPaused = false
			}
		} else {
			task.ActivityAutoPaused = false
			task.ActivityOverride = false
			task.AuthAutoPaused = false
		}
		task.Enabled = body.Enabled
		result := database.DB.Model(&database.Task{}).Where("id = ? AND running_at IS NULL", task.ID).
			Updates(map[string]interface{}{"enabled": task.Enabled, "activity_override": task.ActivityOverride, "activity_auto_paused": task.ActivityAutoPaused, "auth_auto_paused": task.AuthAutoPaused})
		if result.Error != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "更新任务状态失败"})
		}
		if result.RowsAffected == 0 {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "任务正在执行，请稍后重试"})
		}
		return c.JSON(fiber.Map{"message": "任务状态已更新", "task": task})
	})
}
