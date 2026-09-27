package middleware

import (
	"dormcheck/database"

	"github.com/gofiber/fiber/v2"
)

func SuperAdminAuth(c *fiber.Ctx) error {
	currentUser, ok := c.Locals("user").(*database.User)
	if !ok || currentUser.Role != database.RoleSuperAdmin {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "需要超级管理员权限"})
	}
	return c.Next()
}
