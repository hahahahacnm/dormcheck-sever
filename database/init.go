package database

import (
	"log"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger" // ✅ 引入 GORM 日志控制器
)

var DB *gorm.DB

func InitDB() {
	var err error

	// ✅ 设置为 Silent，避免误报 record not found
	DB, err = gorm.Open(sqlite.Open("dormcheck.db"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})

	if err != nil {
		log.Fatal("无法连接数据库:", err)
	}

	// 自动迁移表结构
	err = DB.AutoMigrate(
		&User{},
		&UserStudent{},
		&Student{},
		&Task{},
		&EmailVerificationCode{},
		&SponsorActivationCode{},
	)
	if err != nil {
		log.Fatal("数据库迁移失败:", err)
	}

	log.Println("✅ 数据库连接成功，迁移完成")
}
