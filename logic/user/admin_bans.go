package user

import (
	"errors"
	"strings"
	"time"

	"dormcheck/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ListStudentBans(query string, page, pageSize int) ([]database.StudentBan, int64, error) {
	db := database.DB.Model(&database.StudentBan{})
	query = strings.TrimSpace(query)
	if query != "" {
		db = db.Where("stu_id ILIKE ? OR reason ILIKE ?", "%"+query+"%", "%"+query+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var bans []database.StudentBan
	if err := db.Order("banned_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&bans).Error; err != nil {
		return nil, 0, err
	}
	return bans, total, nil
}

func BanStudent(actorID int, stuID string, durationDays int, reason string) (*database.StudentBan, error) {
	stuID = strings.TrimSpace(stuID)
	reason = strings.TrimSpace(reason)
	if stuID == "" || len(stuID) > 64 {
		return nil, errors.New("学号无效")
	}
	if durationDays < 0 || durationDays > 36500 {
		return nil, errors.New("封禁时长无效，天数应为 0 到 36500；0 表示永久")
	}
	if len([]rune(reason)) > 500 {
		return nil, errors.New("封禁原因不能超过 500 个字符")
	}
	now := time.Now()
	ban := &database.StudentBan{StuID: stuID, Reason: reason, BannedAt: now, BannedBy: actorID}
	if durationDays > 0 {
		expiresAt := now.AddDate(0, 0, durationDays)
		ban.ExpiresAt = &expiresAt
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var actor database.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&actor, actorID).Error; err != nil || !database.IsAdminRole(actor.Role) {
			return errors.New("需要管理员权限")
		}
		// Serialize with bind/task creation, both of which lock the student row
		// before checking the ban record.
		var student database.Student
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&student, "stu_id = ?", stuID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "stu_id"}}, DoUpdates: clause.Assignments(map[string]interface{}{
			"reason": reason, "banned_at": now, "expires_at": ban.ExpiresAt, "banned_by": actorID,
		})}).Create(ban).Error; err != nil {
			return err
		}
		// Keep bindings and task settings intact. The scheduler and execution
		// entry points consult this active ban and refuse to run the student.
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ban, nil
}

func UnbanStudent(actorID int, stuID string) error {
	stuID = strings.TrimSpace(stuID)
	if stuID == "" {
		return errors.New("学号无效")
	}
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var actor database.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&actor, actorID).Error; err != nil || !database.IsAdminRole(actor.Role) {
			return errors.New("需要管理员权限")
		}
		return tx.Where("stu_id = ?", stuID).Delete(&database.StudentBan{}).Error
	})
}
