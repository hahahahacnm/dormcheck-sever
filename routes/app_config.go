package routes

import (
	"strings"

	"dormcheck/config"
	"dormcheck/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterAppConfigRoutes(app *fiber.App) {
	app.Get("/app-config/amap", middleware.JwtAuth, func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		key := strings.TrimSpace(config.Get("amap_web_key"))
		securityCode := strings.TrimSpace(config.Get("amap_security_js_code"))
		if key == "" || securityCode == "" {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "地图服务尚未配置，请联系管理员"})
		}
		return c.JSON(fiber.Map{"key": key, "security_code": securityCode})
	})
}
