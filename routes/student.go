// routes/student.go
package routes

import (
	"dormcheck/config"
	"dormcheck/database"
	"dormcheck/external/schoollogin"
	"dormcheck/logic/student"
	"dormcheck/logic/user"
	"dormcheck/middleware"
	"dormcheck/utils"
	"errors"
	"log"
	"math"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

func automaticSignTime(activityRange string) string {
	clocks := regexp.MustCompile(`\d{1,2}:\d{2}(?::\d{2})?`).FindAllString(activityRange, -1)
	if len(clocks) < 2 {
		return ""
	}
	return student.AutomaticSignTime(clocks[0], clocks[1])
}

func validInitialPassword(password string) bool {
	if len([]rune(password)) < 7 {
		return false
	}
	var upper, lower, digit bool
	for _, r := range password {
		switch {
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= '0' && r <= '9':
			digit = true
		}
	}
	return (upper && lower) || (upper && digit) || (lower && digit)
}

func validNotifyEmail(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	if strings.ContainsAny(value, "\r\n,") {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value
}

func validCoordinates(longitude, latitude float64) bool {
	return !math.IsNaN(longitude) && !math.IsNaN(latitude) &&
		!math.IsInf(longitude, 0) && !math.IsInf(latitude, 0) &&
		longitude >= -180 && longitude <= 180 && latitude >= -90 && latitude <= 90 &&
		longitude != 0 && latitude != 0
}

// RegisterStudentRoutes 注册与学生相关的接口路由
func RegisterStudentRoutes(app *fiber.App) {
	studentGroup := app.Group("/student", middleware.JwtAuth)
	studentRateKey := func(c *fiber.Ctx) string { return strconv.Itoa(c.Locals("userID").(int)) }
	bindLimiter := middleware.RateLimit(20, time.Hour, studentRateKey)
	activityLimiter := middleware.RateLimit(60, time.Minute, studentRateKey)
	manualLimiter := middleware.RateLimit(10, time.Minute, studentRateKey)

	// 用户绑定学号
	studentGroup.Post("/bind", middleware.UserNotBanned, bindLimiter, func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(int)

		var data struct {
			StuID    string `json:"stu_id"`
			Password string `json:"password"`
		}
		if err := c.BodyParser(&data); err != nil || data.StuID == "" || data.Password == "" {
			log.Printf("绑定请求参数错误: 用户ID=%d", userID)
			return utils.RespondJSON(c, 400, false, "参数错误，学号和密码为必填项", nil)
		}

		log.Printf("收到用户绑定请求: 用户ID=%d, 学生ID=%s", userID, data.StuID)

		err := student.LoginAndBindStudent(userID, data.StuID, data.Password)
		if err != nil {
			log.Printf("绑定失败，错误信息: %v", err)

			// 尝试获取 g 值（只有特定登录失败情况有）
			var gValue string
			if le, ok := err.(*student.LoginFailWithG); ok {
				gValue = le.G
			}

			respData := map[string]string{}
			if gValue != "" {
				respData["g"] = gValue
			} else {
				respData = nil
			}

			return utils.RespondJSON(c, 400, false, "绑定失败: "+err.Error(), respData)
		}

		return utils.RespondJSON(c, 200, true, "绑定成功", nil)
	})

	// 在本站提交微学工初始密码修改，不向前端暴露或记录密码密文。
	studentGroup.Post("/change-initial-password", middleware.UserNotBanned, func(c *fiber.Ctx) error {
		var data struct {
			StuID       string `json:"stu_id"`
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
			G           string `json:"g"`
		}
		if err := c.BodyParser(&data); err != nil || data.StuID == "" || data.OldPassword == "" || data.NewPassword == "" || data.G == "" {
			return utils.RespondJSON(c, 400, false, "学号、原密码、新密码和平台凭证均为必填项", nil)
		}
		if !validInitialPassword(data.NewPassword) {
			return utils.RespondJSON(c, 400, false, "新密码至少 7 位，并需包含大小写字母或字母与数字组合", nil)
		}
		if err := schoollogin.ChangeInitialPassword(data.G, data.OldPassword, data.NewPassword); err != nil {
			return utils.RespondJSON(c, 400, false, "微学工密码修改失败: "+err.Error(), nil)
		}
		return utils.RespondJSON(c, 200, true, "微学工密码修改成功", nil)
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

	// 查询单个已绑定学生的密码
	studentGroup.Get("/password/:stu_id", func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		userID := c.Locals("userID").(int)
		stuID := c.Params("stu_id")

		if stuID == "" {
			return utils.RespondJSON(c, 400, false, "参数错误，stu_id 不能为空", nil)
		}

		// 查询密码（仅限用户自己已绑定的学号）
		pwd, err := user.GetStudentPassword(userID, stuID)
		if err != nil {
			return utils.RespondJSON(c, 400, false, "查询失败: "+err.Error(), nil)
		}

		// 返回学号与密码
		return utils.RespondJSON(c, 200, true, "查询成功", fiber.Map{
			"stu_id":   stuID,
			"password": pwd,
		})
	})

	// 从微学工查询指定学生的签到活动列表
	studentGroup.Get("/activities", activityLimiter, func(c *fiber.Ctx) error {
		stuID := c.Query("stu_id")
		if stuID == "" {
			return utils.RespondJSON(c, 400, false, "缺少参数 stu_id", nil)
		}
		if err := student.EnsureStudentNotBanned(stuID); err != nil {
			return utils.RespondJSON(c, 403, false, err.Error(), nil)
		}
		bound, err := student.UserHasStudentBinding(c.Locals("userID").(int), stuID)
		if err != nil {
			return utils.RespondJSON(c, 500, false, "检查学生绑定失败", nil)
		}
		if !bound {
			return utils.RespondJSON(c, 403, false, "请先绑定该学生账号", nil)
		}

		activities, err := student.GetStudentActivityList(stuID)
		if err != nil {
			return utils.RespondJSON(c, 500, false, "获取活动失败: "+err.Error(), nil)
		}

		return utils.RespondJSON(c, 200, true, "查询成功", activities)
	})

	// 添加签到任务
	studentGroup.Post("/task", middleware.UserNotBanned, func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(int)

		var data struct {
			StuID             string  `json:"stu_id"`
			ActivityID        string  `json:"activity_id"`
			Name              string  `json:"name"`          // 学生姓名
			ActivityName      string  `json:"activity_name"` // 活动名称
			ActivityCollege   string  `json:"activity_college"`
			SignMode          string  `json:"sign_mode"`
			ActivityTimeRange string  `json:"activity_time_range"`
			ActivityStartDate string  `json:"activity_start_date"`
			ActivityEndDate   string  `json:"activity_end_date"`
			SignType          int     `json:"sign_type"`
			QRCodeType        int     `json:"qrcode_type"`
			Address           string  `json:"address"`
			Longitude         float64 `json:"longitude"`
			Latitude          float64 `json:"latitude"`
			SignTime          string  `json:"sign_time"` // 忽略客户端值，由活动开始时间推导
			MaxRetry          int     `json:"max_retry"`
			NotifyEmail       string  `json:"notify_email"` // ✅ 新增：通知邮箱
		}

		if err := c.BodyParser(&data); err != nil {
			return utils.RespondJSON(c, 400, false, "请求体解析失败", nil)
		}
		data.NotifyEmail = strings.TrimSpace(data.NotifyEmail)
		if data.StuID == "" || data.ActivityID == "" {
			return utils.RespondJSON(c, 400, false, "学生账号和活动 ID 不能为空", nil)
		}
		if data.Address == "" || !validCoordinates(data.Longitude, data.Latitude) || !validNotifyEmail(data.NotifyEmail) {
			return utils.RespondJSON(c, 400, false, "签到地点、坐标或通知邮箱无效", nil)
		}
		if err := student.EnsureStudentNotBanned(data.StuID); err != nil {
			return utils.RespondJSON(c, 403, false, err.Error(), nil)
		}
		bound, err := student.UserHasStudentBinding(userID, data.StuID)
		if err != nil {
			return utils.RespondJSON(c, 500, false, "检查学生绑定失败", nil)
		}
		if !bound {
			return utils.RespondJSON(c, 403, false, "请先绑定该学生账号", nil)
		}

		activities, err := student.GetStudentActivityList(data.StuID)
		if err != nil {
			return utils.RespondJSON(c, 400, false, "读取上游活动属性失败: "+err.Error(), nil)
		}
		found := false
		for _, activity := range activities {
			if strconv.Itoa(activity.ID) != data.ActivityID {
				continue
			}
			found = true
			data.ActivityName, data.ActivityCollege = activity.Name, activity.Collegeview
			data.SignType, data.QRCodeType = activity.SignType, activity.QRCodeType
			data.ActivityStartDate, data.ActivityEndDate = activity.ForeachpStartday, activity.ForeachpEndday
			start, end := activity.ForeachpStarttime, activity.ForeachpEndtime
			if start == "" {
				start = activity.DesignateStarttime
			}
			if end == "" {
				end = activity.DesignateEndtime
			}
			data.ActivityTimeRange = start + "-" + end
			break
		}
		if !found {
			return utils.RespondJSON(c, 400, false, "所选活动已不存在或不属于该学生账号", nil)
		}
		if data.SignType != 1 || data.QRCodeType > 0 {
			return utils.RespondJSON(c, 400, false, "当前活动不是受支持的定位签到模式，暂不能建立自动任务", nil)
		}
		if data.ActivityTimeRange == "" || data.ActivityTimeRange[len(data.ActivityTimeRange)-1] == '-' {
			return utils.RespondJSON(c, 400, false, "活动未返回完整签到时段", nil)
		}
		data.SignTime = automaticSignTime(data.ActivityTimeRange)
		if data.SignTime == "" {
			return utils.RespondJSON(c, 400, false, "活动未返回有效的开始时间，无法自动安排任务", nil)
		}
		task := &database.Task{
			UserID:            userID,
			StuID:             data.StuID,
			ActivityID:        data.ActivityID,
			Name:              data.Name,
			ActivityName:      data.ActivityName,
			ActivityCollege:   data.ActivityCollege,
			SignMode:          data.SignMode,
			ActivityTimeRange: data.ActivityTimeRange,
			ActivityStartDate: data.ActivityStartDate,
			ActivityEndDate:   data.ActivityEndDate,
			SignType:          data.SignType,
			QRCodeType:        data.QRCodeType,
			Address:           data.Address,
			Longitude:         data.Longitude,
			Latitude:          data.Latitude,
			SignTime:          data.SignTime,
			MaxRetry:          config.GetInt("task_max_retries", 3) + 1,
			NotifyEmail:       data.NotifyEmail, // ✅ 新增：赋值邮箱
			Enabled:           true,
			ExecStatus:        "pending",
		}

		if err := student.SaveTask(userID, task); err != nil {
			if err.Error() == "permission denied" {
				return utils.RespondJSON(c, 403, false, "请先绑定该学生账号", nil)
			}
			return utils.RespondJSON(c, 400, false, "保存失败: "+err.Error(), nil)
		}

		return utils.RespondJSON(c, 200, true, "任务保存成功", nil)
	})

	// 删除指定签到任务
	studentGroup.Post("/task/delete", middleware.UserNotBanned, func(c *fiber.Ctx) error {
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

		bound, err := student.UserHasStudentBinding(userID, task.StuID)
		if err != nil || !bound {
			return utils.RespondJSON(c, 403, false, "请先绑定该学生账号，才能操作此共享任务", nil)
		}

		result := database.DB.Where("id = ? AND running_at IS NULL AND stu_id IN (SELECT stu_id FROM user_students WHERE user_id = ?)", task.ID, userID).Delete(&database.Task{})
		if result.Error != nil {
			return utils.RespondJSON(c, 500, false, "删除失败: "+result.Error.Error(), nil)
		}
		if result.RowsAffected == 0 {
			return utils.RespondJSON(c, 409, false, "任务正在执行或绑定关系已变化，请稍后重试", nil)
		}

		return utils.RespondJSON(c, 200, true, "任务删除成功", nil)
	})

	// 修改指定签到任务
	studentGroup.Post("/task/update", middleware.UserNotBanned, func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(int)

		var data struct {
			TaskID            uint    `json:"task_id"`
			StuID             string  `json:"stu_id"`
			ActivityID        string  `json:"activity_id"`
			Name              string  `json:"name"`
			ActivityName      string  `json:"activity_name"`
			ActivityCollege   string  `json:"activity_college"`
			SignMode          string  `json:"sign_mode"`
			ActivityTimeRange string  `json:"activity_time_range"`
			ActivityStartDate string  `json:"activity_start_date"`
			ActivityEndDate   string  `json:"activity_end_date"`
			SignType          int     `json:"sign_type"`
			QRCodeType        int     `json:"qrcode_type"`
			Address           string  `json:"address"`
			Longitude         float64 `json:"longitude"`
			Latitude          float64 `json:"latitude"`
			SignTime          string  `json:"sign_time"`
			MaxRetry          int     `json:"max_retry"`
			NotifyEmail       string  `json:"notify_email"`
		}

		if err := c.BodyParser(&data); err != nil || data.TaskID == 0 {
			return utils.RespondJSON(c, 400, false, "参数错误，task_id 不能为空", nil)
		}
		data.NotifyEmail = strings.TrimSpace(data.NotifyEmail)

		if data.Address == "" || !validCoordinates(data.Longitude, data.Latitude) || !validNotifyEmail(data.NotifyEmail) {
			return utils.RespondJSON(c, 400, false, "签到地点、坐标或通知邮箱无效", nil)
		}
		var existing database.Task
		if err := database.DB.First(&existing, data.TaskID).Error; err != nil {
			return utils.RespondJSON(c, 404, false, "任务不存在", nil)
		}
		if err := student.EnsureStudentNotBanned(existing.StuID); err != nil {
			return utils.RespondJSON(c, 403, false, err.Error(), nil)
		}
		bound, err := student.UserHasStudentBinding(userID, existing.StuID)
		if err != nil {
			return utils.RespondJSON(c, 500, false, "检查学生绑定失败", nil)
		}
		if !bound {
			return utils.RespondJSON(c, 403, false, "无权操作此任务", nil)
		}
		if data.StuID != existing.StuID || data.ActivityID != existing.ActivityID {
			return utils.RespondJSON(c, 400, false, "不能将共享任务转移到其他学生或活动", nil)
		}

		activities, err := student.GetStudentActivityList(data.StuID)
		if err != nil {
			return utils.RespondJSON(c, 400, false, "读取上游活动属性失败: "+err.Error(), nil)
		}
		found := false
		for _, activity := range activities {
			if strconv.Itoa(activity.ID) != data.ActivityID {
				continue
			}
			found = true
			data.ActivityName, data.ActivityCollege = activity.Name, activity.Collegeview
			data.SignType, data.QRCodeType = activity.SignType, activity.QRCodeType
			data.ActivityStartDate, data.ActivityEndDate = activity.ForeachpStartday, activity.ForeachpEndday
			start, end := activity.ForeachpStarttime, activity.ForeachpEndtime
			if start == "" {
				start = activity.DesignateStarttime
			}
			if end == "" {
				end = activity.DesignateEndtime
			}
			data.ActivityTimeRange = start + "-" + end
			break
		}
		if !found {
			return utils.RespondJSON(c, 400, false, "所选活动已不存在或不属于该学生账号", nil)
		}
		if data.SignType != 1 || data.QRCodeType > 0 {
			return utils.RespondJSON(c, 400, false, "当前活动不是受支持的定位签到模式，暂不能更新自动任务", nil)
		}
		if data.ActivityTimeRange == "" || data.ActivityTimeRange[len(data.ActivityTimeRange)-1] == '-' {
			return utils.RespondJSON(c, 400, false, "活动未返回完整签到时段", nil)
		}
		data.SignTime = automaticSignTime(data.ActivityTimeRange)
		if data.SignTime == "" {
			return utils.RespondJSON(c, 400, false, "活动未返回有效的开始时间，无法自动安排任务", nil)
		}

		updated, err := student.UpdateTask(data.TaskID, userID, &database.Task{
			StuID:             data.StuID,
			ActivityID:        data.ActivityID,
			Name:              data.Name,
			ActivityName:      data.ActivityName,
			ActivityCollege:   data.ActivityCollege,
			SignMode:          data.SignMode,
			ActivityTimeRange: data.ActivityTimeRange,
			ActivityStartDate: data.ActivityStartDate,
			ActivityEndDate:   data.ActivityEndDate,
			SignType:          data.SignType,
			QRCodeType:        data.QRCodeType,
			Address:           data.Address,
			Longitude:         data.Longitude,
			Latitude:          data.Latitude,
			SignTime:          data.SignTime,
			MaxRetry:          config.GetInt("task_max_retries", 3) + 1,
			NotifyEmail:       data.NotifyEmail,
		})
		if err != nil {
			if err.Error() == "permission denied" {
				return utils.RespondJSON(c, 403, false, "当前用户没有权限修改该任务", nil)
			}
			return utils.RespondJSON(c, 400, false, "更新失败: "+err.Error(), nil)
		}

		return utils.RespondJSON(c, 200, true, "任务更新成功", updated)
	})

	// 查询当前用户的所有签到任务
	studentGroup.Get("/tasks", func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(int)

		var tasks []database.Task
		if err := database.DB.Where("stu_id IN (?)", database.DB.Model(&database.UserStudent{}).Select("stu_id").Where("user_id = ?", userID)).Find(&tasks).Error; err != nil {
			return utils.RespondJSON(c, 500, false, "查询任务失败: "+err.Error(), nil)
		}
		if err := database.AttachActiveStudentBans(tasks, time.Now()); err != nil {
			return utils.RespondJSON(c, 500, false, "查询学生封禁状态失败", nil)
		}

		return utils.RespondJSON(c, 200, true, "查询成功", tasks)
	})

	// 暂停/恢复任务
	studentGroup.Post("/task/toggle", middleware.UserNotBanned, func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(int)

		var data struct {
			TaskID  uint `json:"task_id"`
			Enabled bool `json:"enabled"` // true=启用，false=暂停
		}
		if err := c.BodyParser(&data); err != nil || data.TaskID == 0 {
			return utils.RespondJSON(c, 400, false, "参数错误，task_id 不能为空", nil)
		}

		var task database.Task
		if err := database.DB.First(&task, data.TaskID).Error; err != nil {
			return utils.RespondJSON(c, 404, false, "任务不存在", nil)
		}

		bound, err := student.UserHasStudentBinding(userID, task.StuID)
		if err != nil || !bound {
			return utils.RespondJSON(c, 403, false, "请先绑定该学生账号，才能操作此共享任务", nil)
		}
		if data.Enabled {
			if err := student.EnsureStudentNotBanned(task.StuID); err != nil {
				return utils.RespondJSON(c, 403, false, err.Error(), nil)
			}
			if err := student.EnsureStudentCanEnableTasks(task.StuID); err != nil {
				return utils.RespondJSON(c, 400, false, err.Error(), nil)
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

		task.Enabled = data.Enabled
		result := database.DB.Model(&database.Task{}).
			Where("id = ? AND running_at IS NULL AND stu_id IN (SELECT stu_id FROM user_students WHERE user_id = ?)", task.ID, userID).
			Updates(map[string]interface{}{"enabled": task.Enabled, "activity_auto_paused": task.ActivityAutoPaused, "activity_override": task.ActivityOverride, "auth_auto_paused": task.AuthAutoPaused})
		if result.Error != nil {
			return utils.RespondJSON(c, 500, false, "更新失败: "+result.Error.Error(), nil)
		}
		if result.RowsAffected == 0 {
			return utils.RespondJSON(c, 409, false, "任务正在执行或绑定关系已变化，请稍后重试", nil)
		}

		action := "暂停"
		if data.Enabled {
			action = "恢复"
		}

		// ✅ 返回任务ID在data里
		return utils.RespondJSON(c, 200, true, "任务已"+action, map[string]uint{
			"task_id": task.ID,
		})
	})

	studentGroup.Post("/task/:id/run", middleware.UserNotBanned, manualLimiter, func(c *fiber.Ctx) error {
		taskID64, err := strconv.ParseUint(c.Params("id"), 10, 32)
		if err != nil || taskID64 == 0 {
			return utils.RespondJSON(c, 400, false, "任务 ID 无效", nil)
		}
		task, err := student.RunTaskNow(uint(taskID64), c.Locals("userID").(int), false)
		if err != nil {
			if errors.Is(err, student.ErrStudentBanned) {
				return utils.RespondJSON(c, 403, false, err.Error(), nil)
			}
			if err == student.ErrManualRunOutsideWindow {
				return utils.RespondJSON(c, 400, false, err.Error(), nil)
			}
			if errors.Is(err, student.ErrTaskAlreadyRunning) {
				return utils.RespondJSON(c, 409, false, err.Error(), nil)
			}
			if errors.Is(err, student.ErrTaskResultNotSaved) {
				return utils.RespondJSON(c, 500, false, err.Error(), nil)
			}
			if err.Error() == "permission denied" {
				return utils.RespondJSON(c, 403, false, "无权操作此任务", nil)
			}
			return utils.RespondJSON(c, 404, false, "任务不存在或执行状态读取失败", nil)
		}
		return utils.RespondJSON(c, 200, true, "手动签到已执行", task)
	})

}
