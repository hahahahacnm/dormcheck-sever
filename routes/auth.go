package routes

import (
	"dormcheck/logic/user"
	"dormcheck/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterAuthRoutes(app *fiber.App) {
	auth := app.Group("/auth")

	// 发送验证码接口
	auth.Post("/send-code", func(c *fiber.Ctx) error {
		var data struct {
			Email   string `json:"email"`
			Purpose string `json:"purpose"`
		}
		if err := c.BodyParser(&data); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "参数错误"})
		}

		if data.Email == "" || data.Purpose == "" {
			return c.Status(400).JSON(fiber.Map{"error": "邮箱和用途不能为空"})
		}

		var err error
		switch data.Purpose {
		case "register":
			err = user.SendRegisterVerificationCode(data.Email)
		case "change_email":
			err = user.SendChangeEmailVerificationCode(data.Email)
		case "reset":
			err = user.SendResetPasswordCode(data.Email) // 新增用途 reset
		default:
			return c.Status(400).JSON(fiber.Map{"error": "无效的用途"})
		}

		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"message": "验证码发送成功"})
	})

	// 注册接口
	auth.Post("/register", func(c *fiber.Ctx) error {
		var data struct {
			Username string `json:"username"`
			Email    string `json:"email"`
			Password string `json:"password"`
			Code     string `json:"code"` // 验证码
		}
		if err := c.BodyParser(&data); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "参数错误"})
		}

		if data.Username == "" || data.Email == "" || data.Password == "" || data.Code == "" {
			return c.Status(400).JSON(fiber.Map{"error": "用户名、邮箱、密码和验证码不能为空"})
		}

		if len(data.Username) < 3 || len(data.Password) < 6 {
			return c.Status(400).JSON(fiber.Map{"error": "用户名至少3个字符，密码至少6个字符"})
		}

		if err := user.Register(data.Username, data.Email, data.Password, data.Code); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"message": "注册成功"})
	})

	// 登录
	auth.Post("/login", func(c *fiber.Ctx) error {
		var data struct {
			Identifier string `json:"username"` // 用户名或邮箱，前端字段仍用 username
			Password   string `json:"password"`
		}
		if err := c.BodyParser(&data); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "参数错误"})
		}

		if data.Identifier == "" || data.Password == "" {
			return c.Status(400).JSON(fiber.Map{"error": "用户名/邮箱和密码不能为空"})
		}

		token, err := user.Login(data.Identifier, data.Password)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"token": token})
	})

	// 忘记密码重置接口（重置后强制下线）
	auth.Post("/reset-password", func(c *fiber.Ctx) error {
		var data struct {
			Email       string `json:"email"`
			Code        string `json:"code"`
			NewPassword string `json:"new_password"`
		}
		if err := c.BodyParser(&data); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "参数错误"})
		}

		if data.Email == "" || data.Code == "" || data.NewPassword == "" {
			return c.Status(400).JSON(fiber.Map{"error": "邮箱、验证码和新密码不能为空"})
		}

		err := user.ResetPasswordAndForceLogout(data.Email, data.Code, data.NewPassword)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"message": "密码重置成功，已强制下线所有设备，请使用新密码重新登录"})
	})

	// 登出
	auth.Post("/logout", middleware.JwtAuth, func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(int)
		if err := user.ForceLogoutAll(userID); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "退出失败"})
		}
		return c.JSON(fiber.Map{"message": "已强制下线所有设备"})
	})

	// 获取当前用户信息
	auth.Get("/me", middleware.JwtAuth, func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(int)

		u, err := user.GetUserByID(userID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "获取用户信息失败"})
		}

		return c.JSON(fiber.Map{
			"id":       u.ID,
			"username": u.Username,
			"role":     u.Role,
			"email":    u.Email,
		})
	})

	// 密码修改（修改成功后强制登出所有设备）
	auth.Post("/change-password", middleware.JwtAuth, func(c *fiber.Ctx) error {
		var body struct {
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"message": "参数错误"})
		}

		userID := c.Locals("userID").(int)

		u, err := user.GetUserByID(userID)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"message": "用户不存在"})
		}

		if !user.CheckPasswordHash(body.OldPassword, u.Password) {
			return c.Status(400).JSON(fiber.Map{"message": "原密码错误"})
		}

		newHash, err := user.HashPassword(body.NewPassword)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"message": "密码加密失败"})
		}

		if err := user.UpdateUserPassword(u.ID, newHash); err != nil {
			return c.Status(500).JSON(fiber.Map{"message": "密码更新失败"})
		}

		// ✅ 修改密码后强制所有设备下线
		if err := user.ForceLogoutAll(u.ID); err != nil {
			// 这里建议只打印日志，不中断流程
			// log.Warnf("用户 %d 修改密码后强制登出失败: %v", u.ID, err)
		}

		return c.JSON(fiber.Map{"message": "密码修改成功，已强制下线所有设备，请重新登录"})
	})

	// 邮箱修改
	auth.Post("/change-email", middleware.JwtAuth, func(c *fiber.Ctx) error {
		var body struct {
			NewEmail string `json:"new_email"`
			Code     string `json:"code"`
		}

		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"message": "参数错误"})
		}

		userID := c.Locals("userID").(int)

		if err := user.ChangeUserEmail(userID, body.NewEmail, body.Code); err != nil {
			return c.Status(400).JSON(fiber.Map{"message": err.Error()})
		}

		return c.JSON(fiber.Map{"message": "邮箱修改成功，请验证新邮箱"})
	})

	auth.Post("/send-change-email-code", func(c *fiber.Ctx) error {
		var body struct {
			Email string `json:"email"`
		}
		if err := c.BodyParser(&body); err != nil || body.Email == "" {
			return c.Status(400).JSON(fiber.Map{"message": "邮箱不能为空"})
		}

		if err := user.SendChangeEmailVerificationCode(body.Email); err != nil {
			return c.Status(400).JSON(fiber.Map{"message": err.Error()})
		}

		return c.JSON(fiber.Map{"message": "验证码发送成功"})
	})

	// 激活码赞助接口
	auth.Post("/sponsor-activate", middleware.JwtAuth, func(c *fiber.Ctx) error {
		var body struct {
			Code string `json:"code"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"message": "参数错误"})
		}

		if body.Code == "" {
			return c.Status(400).JSON(fiber.Map{"message": "激活码不能为空"})
		}

		userID, ok := c.Locals("userID").(int)
		if !ok {
			return c.Status(401).JSON(fiber.Map{"message": "未登录"})
		}

		if err := user.UseSponsorCode(userID, body.Code); err != nil {
			return c.Status(400).JSON(fiber.Map{"message": err.Error()})
		}

		return c.JSON(fiber.Map{"message": "激活成功，感谢您的支持！"})
	})

}
