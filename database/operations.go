package database

import (
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetDB returns the PostgreSQL connection initialized by InitDB.
func GetDB() *gorm.DB {
	if DB == nil {
		InitDB()
	}
	return DB
}

// SaveStudentOrUpdate 保存或更新学生信息
func SaveStudentOrUpdate(student *Student) error {
	db := GetDB()

	var existing Student
	err := db.First(&existing, "stu_id = ?", student.StuID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			if student.AuthStatus == "" {
				student.AuthStatus = "valid"
			}
			log.Println("未找到学生记录，准备插入新的学生信息:", student.StuID)
			if err := db.Create(student).Error; err != nil {
				return fmt.Errorf("插入学生信息失败: %v", err)
			}
			return nil
		}
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var current Student
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "stu_id = ?", student.StuID).Error; err != nil {
			return err
		}
		current.Password = student.Password
		current.Cookies = student.Cookies
		current.LastLogin = time.Now()
		current.Name = student.Name
		current.AuthStatus = "valid"
		current.AuthFailedAt = nil
		current.AuthError = ""
		current.AuthNoticeSentAt = nil
		if err := tx.Save(&current).Error; err != nil {
			return err
		}
		// Restore only tasks paused by the seven-day credential lock. Activity
		// audit pauses remain in force until the activity itself is healthy.
		if err := tx.Model(&Task{}).
			Where("stu_id = ? AND auth_auto_paused = TRUE AND activity_state = ?", student.StuID, "normal").
			Updates(map[string]interface{}{"enabled": true, "auth_auto_paused": false}).Error; err != nil {
			return err
		}
		log.Println("更新学生信息并恢复账号锁定任务:", student.StuID)
		return nil
	})
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
