// logic/student/task.go
package student

import (
	"dormcheck/database"
	"errors"
	"time"

	"gorm.io/gorm"
)

// SaveTask 尝试保存签到任务，重复可更新
// SaveTask 尝试保存签到任务，重复可更新
func SaveTask(task *database.Task) error {
	// 查找是否存在同 user_id、stu_id、activity_id 的任务
	var existing database.Task
	err := database.DB.
		Where("user_id = ? AND stu_id = ? AND activity_id = ?", task.UserID, task.StuID, task.ActivityID).
		First(&existing).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 不存在，新增任务
			task.Enabled = true
			task.ExecStatus = "pending"
			task.RetryCount = 0
			task.ExecutedAt = time.Time{}

			return database.DB.Create(task).Error
		}
		// 查询失败
		return err
	}

	// 找到已有任务，更新字段（除主键外）
	existing.Address = task.Address
	existing.Longitude = task.Longitude
	existing.Latitude = task.Latitude
	existing.SignTime = task.SignTime
	existing.MaxRetry = task.MaxRetry
	existing.Enabled = true
	existing.Name = task.Name
	existing.ActivityName = task.ActivityName
	existing.NotifyEmail = task.NotifyEmail // ✅ 新增：更新邮箱字段

	// 重置状态
	existing.ExecStatus = "pending"
	existing.RetryCount = 0
	existing.LastError = ""
	existing.ExecutedAt = time.Time{}

	return database.DB.Save(&existing).Error
}
