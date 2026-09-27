package database

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger" // ✅ 引入 GORM 日志控制器
)

var DB *gorm.DB

func InitDB() {
	dsn, err := postgresDSN()
	if err != nil {
		log.Fatal("数据库配置无效:", err)
	}

	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})

	if err != nil {
		log.Fatal("无法连接数据库:", err)
	}
	if err := migrateTasksToSharedStudentScope(); err != nil {
		log.Fatal("签到任务共享迁移失败:", err)
	}
	if err := migrateDuplicateStudentBindings(); err != nil {
		log.Fatal("学生绑定去重迁移失败:", err)
	}

	// 自动迁移表结构
	err = DB.AutoMigrate(
		&User{},
		&UserStudent{},
		&Student{},
		&Task{},
		&EmailVerificationCode{},
		&SponsorActivationCode{},
		&SystemSetting{},
		&PlatformPost{},
		&StudentBan{},
	)
	if err != nil {
		log.Fatal("数据库迁移失败:", err)
	}

	log.Println("✅ 数据库连接成功，迁移完成")
}

func postgresDSN() (string, error) {
	host := envOrDefault("PGHOST", "127.0.0.1")
	port := envOrDefault("PGPORT", "5432")
	user := envOrDefault("PGUSER", "postgres")
	database := envOrDefault("PGDATABASE", "dormcheck")
	password := os.Getenv("PGPASSWORD")
	sslmode := envOrDefault("PGSSLMODE", "disable")

	if _, err := strconv.Atoi(port); err != nil {
		return "", fmt.Errorf("PGPORT 必须是有效端口号: %w", err)
	}
	if password == "" {
		return "", fmt.Errorf("请在 .env 或系统环境变量中设置 PostgreSQL 密码（PGPASSWORD）")
	}

	dsnURL := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + database,
	}
	query := dsnURL.Query()
	query.Set("sslmode", sslmode)
	dsnURL.RawQuery = query.Encode()
	return dsnURL.String(), nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
