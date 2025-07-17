package main

import (
	"dormcheck/config"
	"dormcheck/database"
	"dormcheck/logger" // ✅ 添加这一行
	"dormcheck/routes"
	"dormcheck/scheduler" // ✅ 引入调度器
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func main() {
	logger.InitLogger() // ✅ 初始化日志模块
	config.InitConfig() // ✅ 载入环境配置
	database.InitDB()   // ✅ 初始化数据库

	app := fiber.New()
	// ✅ 添加 CORS 中间件
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*", // 可以指定为 http://localhost:5173 更安全
		AllowMethods: "GET,POST,OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	routes.RegisterAuthRoutes(app)
	routes.RegisterStudentRoutes(app)

	// ✅ 启动自动任务调度器 & 每日重置器（必须在主线程之外执行）
	go scheduler.StartWorker()
	go scheduler.StartResetWorker()
	go scheduler.StartCookieRefresher()

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("DormCheck 后端服务已启动！")
	})

	if err := app.Listen(":8080"); err != nil {
		log.Fatal(err)
	}
	fmt.Println("✅ DormCheck 启动成功")
}
