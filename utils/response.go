package utils

import "github.com/gofiber/fiber/v2"

// RespondJSON 返回标准格式的 JSON 响应
func RespondJSON(c *fiber.Ctx, status int, success bool, message string, data any) error {
	return c.Status(status).JSON(fiber.Map{
		"success": success,
		"message": message,
		"data":    data,
	})
}
