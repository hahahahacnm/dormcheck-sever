package database

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 数据库连接单例变量
var dbInstance *gorm.DB

// GetDB 返回数据库连接实例，使用 SQLite 数据库
func GetDB() *gorm.DB {
	if dbInstance == nil {
		var err error
		dbInstance, err = gorm.Open(sqlite.Open("dormcheck.db"), &gorm.Config{})
		if err != nil {
			panic(fmt.Sprintf("连接数据库失败: %v", err))
		}

		// 自动迁移模型，新增 Announcement
		err = dbInstance.AutoMigrate(&User{}, &UserStudent{}, &Student{}, &Task{}, &EmailVerificationCode{}, &SponsorActivationCode{})
		if err != nil {
			panic(fmt.Sprintf("自动迁移失败: %v", err))
		}
	}
	return dbInstance
}

// SaveStudentOrUpdate 保存或更新学生信息
func SaveStudentOrUpdate(student *Student) error {
	db := GetDB()

	var existing Student
	err := db.First(&existing, "stu_id = ?", student.StuID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Println("未找到学生记录，准备插入新的学生信息:", student.StuID)
			if err := db.Create(student).Error; err != nil {
				return fmt.Errorf("插入学生信息失败: %v", err)
			}
			return nil
		}
		return err
	}

	existing.Password = student.Password
	existing.Cookies = student.Cookies
	existing.LastLogin = time.Now()
	existing.Name = student.Name

	log.Println("更新学生信息:", student.StuID)
	return db.Save(&existing).Error
}

// BindUserAndStudent 绑定用户与学生学号，支持同时写入学生姓名
func BindUserAndStudent(userID int, stuID string, name string) error {
	db := GetDB()

	var existing UserStudent
	err := db.First(&existing, "user_id = ? AND stu_id = ?", userID, stuID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			binding := UserStudent{
				UserID: userID,
				StuID:  stuID,
				Name:   name,
			}
			return db.Create(&binding).Error
		}
		return err
	}

	log.Println("✅ 该用户已绑定该学生，无需重复绑定")
	return nil
}

// GetStudentByStuID 根据学号查找 student 信息，使用 GetDB()
func GetStudentByStuID(stuID string) (*Student, error) {
	db := GetDB()
	var student Student
	if err := db.Where("stu_id = ?", stuID).First(&student).Error; err != nil {
		return nil, err
	}
	return &student, nil
}
