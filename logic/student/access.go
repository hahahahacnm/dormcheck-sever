package student

import (
	"dormcheck/database"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"time"
)

var ErrStudentBanned = errors.New("student account is banned")

type studentBannedError struct{ message string }

func (e studentBannedError) Error() string        { return e.message }
func (e studentBannedError) Is(target error) bool { return target == ErrStudentBanned }

func EnsureStudentNotBanned(stuID string) error {
	ban, err := database.GetActiveStudentBan(database.DB, stuID, time.Now())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("检查学生账号封禁状态失败: %w", err)
	}
	return studentBannedError{message: database.StudentBanMessage(*ban)}
}

func UserHasStudentBinding(userID int, stuID string) (bool, error) {
	var count int64
	err := database.DB.Model(&database.UserStudent{}).
		Where("user_id = ? AND stu_id = ?", userID, stuID).
		Count(&count).Error
	return count > 0, err
}

func EnsureStudentCanEnableTasks(stuID string) error {
	account, err := database.GetStudentByStuID(stuID)
	if err != nil {
		return errors.New("学生账号记录不存在，请重新验证绑定")
	}
	if account.AuthStatus == "locked" {
		return errors.New("学生账号已锁定自动任务，请更新密码并重新验证绑定")
	}
	if account.AuthStatus == "invalid" {
		if account.AuthFailedAt == nil || !time.Now().Before(account.AuthFailedAt.Add(7*24*time.Hour)) {
			return errors.New("学生账号连续登录失败已达 7 天，任务已锁定；请更新密码并重新验证绑定")
		}
	}
	return nil
}
