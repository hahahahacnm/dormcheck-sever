package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

var (
	JwtSecret       []byte
	DashScopeAPIKey string
)

func InitConfig() {
	// 加载 .env
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️ 未找到 .env 文件，将尝试使用系统环境变量")
	}

	//读取  JWT_SECRET
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("❌ 环境变量 JWT_SECRET 未设置")
	}
	JwtSecret = []byte(secret)

	// 读取 DashScope API Key
	DashScopeAPIKey = os.Getenv("DASHSCOPE_API_KEY")
	if DashScopeAPIKey == "" {
		log.Fatal("❌ 环境变量 DASHSCOPE_API_KEY 未设置")
	}
}
