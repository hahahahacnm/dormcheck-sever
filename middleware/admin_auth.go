package middleware

import (
	"dormcheck/database"

	"github.com/gofiber/fiber/v2"
)

// AdminAuth allows administrators and super administrators to use admin routes.
func AdminAuth(c *fiber.Ctx) error {
	currentUser, ok := c.Locals("user").(*database.User)
	if !ok || !database.IsAdminRole(currentUser.Role) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "需要管理员权限"})
	}
	return c.Next()
}
