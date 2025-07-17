package middleware

import (
	"dormcheck/database"
	"dormcheck/utils"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// JwtAuth 是 Fiber 中间件，用于解析并验证 JWT，并把用户信息放入上下文
func JwtAuth(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(401).JSON(fiber.Map{"error": "缺少 Authorization 认证"})
	}

	// Bearer token 提取
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == authHeader {
		return c.Status(401).JSON(fiber.Map{"error": "Authorization 格式错误，应为 Bearer {token}"})
	}

	// 验证 JWT 有效性并校验 tokenVersion
	claims, err := utils.ParseToken(tokenStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "无效或过期的 token: " + err.Error()})
	}

	// 根据 claims.UserID 查询用户详细信息
	var user database.User
	if err := database.DB.First(&user, claims.UserID).Error; err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "用户不存在"})
	}

	// 将 userID 和完整用户信息存入上下文
	c.Locals("userID", claims.UserID)
	c.Locals("user", &user)

	return c.Next()
}
