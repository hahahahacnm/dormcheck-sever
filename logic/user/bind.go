package user

import (
	"dormcheck/database"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// Unbind a user while shared tasks remain only if another user stays bound.
func UnbindStudent(userID int, stuID string) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var bindings []database.UserStudent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("stu_id = ?", stuID).Find(&bindings).Error; err != nil {
			return err
		}
		found := false
		for _, binding := range bindings {
			if binding.UserID == userID {
				found = true
				break
			}
		}
		if !found {
			return errors.New("绑定关系不存在")
		}
		var taskCount int64
		if err := tx.Model(&database.Task{}).Where("stu_id = ?", stuID).Count(&taskCount).Error; err != nil {
			return err
		}
		if taskCount > 0 && len(bindings) <= 1 {
			return errors.New("该学生目前只有你在使用，删除其所有活动任务后才能解绑")
		}
		return tx.Where("user_id = ? AND stu_id = ?", userID, stuID).Delete(&database.UserStudent{}).Error
	})
}

// 查询用户已绑定的学号列表
type BoundStudentView struct {
	database.UserStudent
	AccountStatus       string     `json:"account_status"`
	AuthFailedAt        *time.Time `json:"auth_failed_at"`
	AuthError           string     `json:"auth_error"`
	StudentBanned       bool       `json:"student_banned"`
	StudentBanReason    string     `json:"student_ban_reason"`
	StudentBanExpiresAt *time.Time `json:"student_ban_expires_at"`
}

func GetBoundStudents(userID int) ([]BoundStudentView, error) {
	var binds []BoundStudentView
	err := database.DB.Table("user_students").
		Select(`user_students.*, COALESCE(students.auth_status, 'valid') AS account_status,
			students.auth_failed_at, students.auth_error,
			(student_bans.stu_id IS NOT NULL) AS student_banned,
			student_bans.reason AS student_ban_reason, student_bans.expires_at AS student_ban_expires_at`).
		Joins("LEFT JOIN students ON students.stu_id = user_students.stu_id").
		Joins("LEFT JOIN student_bans ON student_bans.stu_id = user_students.stu_id AND (student_bans.expires_at IS NULL OR student_bans.expires_at > ?)", time.Now()).
		Where("user_students.user_id = ?", userID).Scan(&binds).Error
	return binds, err
}

// 查询单个学号的密码（仅限用户自己已绑定的学号）
func GetStudentPassword(userID int, stuID string) (string, error) {
	db := database.DB

	// 先确认该用户已经绑定了这个学号
	var bind database.UserStudent
	if err := db.Where("user_id = ? AND stu_id = ?", userID, stuID).First(&bind).Error; err != nil {
		return "", errors.New("学号未绑定或不存在")
	}

	// 查询密码
	var student database.Student
	if err := db.Where("stu_id = ?", stuID).First(&student).Error; err != nil {
		return "", errors.New("学号信息不存在")
	}

	return student.Password, nil
}
