// logic/student/task.go
package student

import (
	"dormcheck/database"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SaveTask 尝试保存签到任务，重复可更新
func SaveTask(userID int, task *database.Task) error {
	if err := EnsureStudentNotBanned(task.StuID); err != nil {
		return err
	}
	// A task is shared by every user bound to this student account.
	if err := EnsureStudentCanEnableTasks(task.StuID); err != nil {
		return err
	}
	task.Enabled = true
	task.ExecStatus = "pending"
	task.ActivityState = "normal"
	task.ActivityIssue = ""
	task.ActivityAutoPaused = false
	task.ActivityOverride = false
	task.RetryCount = 0
	task.LastError = ""
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		var studentRecord database.Student
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&studentRecord, "stu_id = ?", task.StuID).Error; err != nil {
			return err
		}
		if ban, err := database.GetActiveStudentBan(tx, task.StuID, time.Now()); err == nil {
			return errors.New(database.StudentBanMessage(*ban))
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var binding database.UserStudent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND stu_id = ?", userID, task.StuID).First(&binding).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("permission denied")
			}
			return err
		}
		result := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "stu_id"}, {Name: "activity_id"}},
			Where:   clause.Where{Exprs: []clause.Expression{clause.Eq{Column: clause.Column{Table: "tasks", Name: "running_at"}, Value: nil}}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name", "activity_name", "activity_college", "sign_mode", "activity_time_range",
				"activity_start_date", "activity_end_date", "sign_type", "qr_code_type",
				"address", "longitude", "latitude", "sign_time", "notify_email", "max_retry",
			}),
		}).Create(task)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrTaskAlreadyRunning
		}
		return nil
	}); err != nil {
		return err
	}
	return database.DB.Where("stu_id = ? AND activity_id = ?", task.StuID, task.ActivityID).First(task).Error
}

func UpdateTask(taskID uint, userID int, updates *database.Task) (*database.Task, error) {
	var existing database.Task
	if err := database.DB.First(&existing, taskID).Error; err != nil {
		return nil, err
	}
	if err := EnsureStudentNotBanned(existing.StuID); err != nil {
		return nil, err
	}

	var bindCount int64
	if err := database.DB.Model(&database.UserStudent{}).Where("user_id = ? AND stu_id = ?", userID, existing.StuID).Count(&bindCount).Error; err != nil {
		return nil, err
	}
	if bindCount == 0 {
		return nil, errors.New("permission denied")
	}

	if updates.StuID != existing.StuID || updates.ActivityID != existing.ActivityID {
		return nil, errors.New("不能将共享任务转移到其他学生或活动")
	}
	changes := map[string]interface{}{
		"address": updates.Address, "longitude": updates.Longitude, "latitude": updates.Latitude,
		"sign_time": updates.SignTime, "max_retry": updates.MaxRetry, "notify_email": updates.NotifyEmail,
		"activity_name": updates.ActivityName, "activity_college": updates.ActivityCollege,
		"sign_mode": updates.SignMode, "activity_time_range": updates.ActivityTimeRange,
		"activity_start_date": updates.ActivityStartDate, "activity_end_date": updates.ActivityEndDate,
		"sign_type": updates.SignType, "qr_code_type": updates.QRCodeType,
	}
	if updates.Name != "" {
		changes["name"] = updates.Name
	}
	result := database.DB.Model(&database.Task{}).Where("id = ? AND running_at IS NULL AND stu_id IN (SELECT stu_id FROM user_students WHERE user_id = ?)", taskID, userID).Updates(changes)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrTaskAlreadyRunning
	}
	return &existing, database.DB.First(&existing, taskID).Error
}
