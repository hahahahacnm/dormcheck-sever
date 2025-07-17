// routes/student.go
package routes

import (
	"dormcheck/database"
	"dormcheck/logic/student"
	"dormcheck/logic/user"
	"dormcheck/middleware"
	"dormcheck/utils"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
)

// RegisterStudentRoutes 注册与学生相关的接口路由
func RegisterStudentRoutes(app *fiber.App) {
	studentGroup := app.Group("/student", middleware.JwtAuth)

	// 用户绑定学号
	studentGroup.Post("/bind", func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(int)

		var data struct {
			StuID    string `json:"stu_id"`
			Password string `json:"password"`
		}
		if err := c.BodyParser(&data); err != nil || data.StuID == "" || data.Password == "" {
			log.Printf("绑定请求参数错误: %+v", data)
			return utils.RespondJSON(c, 400, false, "参数错误，学号和密码为必填项", nil)
		}

		log.Printf("收到用户绑定请求: 用户ID=%d, 学生ID=%s", userID, data.StuID)

		if err := student.LoginAndBindStudent(userID, data.StuID, data.Password); err != nil {
			log.Printf("绑定失败，错误信息: %v", err)
			return utils.RespondJSON(c, 400, false, "绑定失败: "+err.Error(), nil)
		}

		return utils.RespondJSON(c, 200, true, "绑定成功", nil)
	})

	// 用户解绑学生账号
	studentGroup.Post("/unbind", func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(int)

		var data struct {
			StuID string `json:"stu_id"`
		}
		if err := c.BodyParser(&data); err != nil || data.StuID == "" {
			return utils.RespondJSON(c, 400, false, "参数错误，stu_id 不能为空", nil)
		}

		// 检查该用户绑定的该学号是否还有任务
		hasTasks, err := student.UserHasTasksForStudent(userID, data.StuID)
		if err != nil {
			return utils.RespondJSON(c, 500, false, "检查任务失败: "+err.Error(), nil)
		}
		if hasTasks {
			return utils.RespondJSON(c, 400, false, "解绑失败：该学号下还有未删除的签到任务，请先删除任务后再解绑", nil)
		}

		if err := user.UnbindStudent(userID, data.StuID); err != nil {
			return utils.RespondJSON(c, 400, false, "解绑失败: "+err.Error(), nil)
		}

		return utils.RespondJSON(c, 200, true, "解绑成功", nil)
	})

	// 用户查询已绑定的学生列表
	studentGroup.Get("/list", func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(int)

		binds, err := user.GetBoundStudents(userID)
		if err != nil {
			return utils.RespondJSON(c, 500, false, "查询失败: "+err.Error(), nil)
		}

		return utils.RespondJSON(c, 200, true, "查询成功", binds)
	})

	// 从微学工查询指定学生的签到活动列表
	studentGroup.Get("/activities", func(c *fiber.Ctx) error {
		stuID := c.Query("stu_id")
		if stuID == "" {
			return utils.RespondJSON(c, 400, false, "缺少参数 stu_id", nil)
		}

		activities, err := student.GetStudentActivityList(stuID)
		if err != nil {
			return utils.RespondJSON(c, 500, false, "获取活动失败: "+err.Error(), nil)
		}

		return utils.RespondJSON(c, 200, true, "查询成功", activities)
	})

	// 添加签到任务
	studentGroup.Post("/task", func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(int)

		var data struct {
			StuID        string  `json:"stu_id"`
			ActivityID   string  `json:"activity_id"`
			Name         string  `json:"name"`          // 学生姓名
			ActivityName string  `json:"activity_name"` // 活动名称
			Address      string  `json:"address"`
			Longitude    float64 `json:"longitude"`
			Latitude     float64 `json:"latitude"`
			SignTime     string  `json:"sign_time"` // 格式：HH:mm
			MaxRetry     int     `json:"max_retry"`
			NotifyEmail  string  `json:"notify_email"` // ✅ 新增：通知邮箱
		}

		if err := c.BodyParser(&data); err != nil {
			return utils.RespondJSON(c, 400, false, "请求体解析失败", nil)
		}

		// 校验签到时间格式
		if _, err := time.Parse("15:04", data.SignTime); err != nil {
			return utils.RespondJSON(c, 400, false, "签到时间格式错误，应为 小时:分钟（如：20:30）", nil)
		}

		task := &database.Task{
			UserID:       userID,
			StuID:        data.StuID,
			ActivityID:   data.ActivityID,
			Name:         data.Name,
			ActivityName: data.ActivityName,
			Address:      data.Address,
			Longitude:    data.Longitude,
			Latitude:     data.Latitude,
			SignTime:     data.SignTime,
			MaxRetry:     data.MaxRetry,
			NotifyEmail:  data.NotifyEmail, // ✅ 新增：赋值邮箱
			Enabled:      true,
			ExecStatus:   "pending",
		}

		if err := student.SaveTask(task); err != nil {
			return utils.RespondJSON(c, 400, false, "保存失败: "+err.Error(), nil)
		}

		return utils.RespondJSON(c, 200, true, "任务保存成功", nil)
	})

	// 删除指定签到任务
	studentGroup.Post("/task/delete", func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(int)

		var data struct {
			TaskID uint `json:"task_id"`
		}
		if err := c.BodyParser(&data); err != nil || data.TaskID == 0 {
			return utils.RespondJSON(c, 400, false, "参数错误，task_id 不能为空", nil)
		}

		var task database.Task
		if err := database.DB.First(&task, data.TaskID).Error; err != nil {
			return utils.RespondJSON(c, 404, false, "任务不存在", nil)
		}

		// 确保只能删除自己的任务
		if task.UserID != userID {
			return utils.RespondJSON(c, 403, false, "当前用户不具备操作该任务的权限！", nil)
		}

		if err := database.DB.Delete(&task).Error; err != nil {
			return utils.RespondJSON(c, 500, false, "删除失败: "+err.Error(), nil)
		}

		return utils.RespondJSON(c, 200, true, "任务删除成功", nil)
	})

	// 查询当前用户的所有签到任务
	studentGroup.Get("/tasks", func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(int)

		var tasks []database.Task
		if err := database.DB.Where("user_id = ?", userID).Find(&tasks).Error; err != nil {
			return utils.RespondJSON(c, 500, false, "查询任务失败: "+err.Error(), nil)
		}

		return utils.RespondJSON(c, 200, true, "查询成功", tasks)
	})

}
