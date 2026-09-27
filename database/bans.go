package database

import (
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
)

func IsUserBanActive(user User, now time.Time) bool {
	return user.Banned && (user.BanExpiresAt == nil || now.Before(*user.BanExpiresAt))
}

func UserBanMessage(user User) string {
	period := formatBanPeriod(user.BannedAt, user.BanExpiresAt)
	message := fmt.Sprintf("该账号已被平台封禁（%s）", period)
	if user.BanReason != "" {
		message += "，原因：" + user.BanReason
	}
	return message + "。如有问题，请联系管理员。"
}

func GetActiveStudentBan(db *gorm.DB, stuID string, now time.Time) (*StudentBan, error) {
	var ban StudentBan
	err := db.Where("stu_id = ? AND (expires_at IS NULL OR expires_at > ?)", stuID, now).First(&ban).Error
	if err != nil {
		return nil, err
	}
	return &ban, nil
}

// AttachActiveStudentBans adds active ban details to task API rows without
// persisting those display-only fields in the tasks table.
func AttachActiveStudentBans(tasks []Task, now time.Time) error {
	if len(tasks) == 0 {
		return nil
	}
	stuIDs := make([]string, 0, len(tasks))
	seen := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		if _, ok := seen[task.StuID]; !ok {
			seen[task.StuID] = struct{}{}
			stuIDs = append(stuIDs, task.StuID)
		}
	}
	var bans []StudentBan
	if err := DB.Where("stu_id IN ? AND (expires_at IS NULL OR expires_at > ?)", stuIDs, now).Find(&bans).Error; err != nil {
		return err
	}
	byStudent := make(map[string]StudentBan, len(bans))
	for _, ban := range bans {
		byStudent[ban.StuID] = ban
	}
	for i := range tasks {
		if ban, ok := byStudent[tasks[i].StuID]; ok {
			tasks[i].StudentBanned = true
			tasks[i].StudentBanReason = ban.Reason
			tasks[i].StudentBanExpiresAt = ban.ExpiresAt
		}
	}
	// A shared student's task remains runnable while at least one binding user
	// is not currently banned. Keep the task and bindings intact when none qualify.
	type bindingCounts struct {
		StuID    string
		Total    int64
		Eligible int64
	}
	var counts []bindingCounts
	if err := DB.Table("user_students").Select(`user_students.stu_id, COUNT(*) AS total,
		COUNT(*) FILTER (WHERE users.banned = FALSE OR (users.ban_expires_at IS NOT NULL AND users.ban_expires_at <= ?)) AS eligible`, now).
		Joins("JOIN users ON users.id = user_students.user_id").Where("user_students.stu_id IN ?", stuIDs).
		Group("user_students.stu_id").Scan(&counts).Error; err != nil {
		return err
	}
	countsByStudent := make(map[string]bindingCounts, len(counts))
	for _, row := range counts {
		countsByStudent[row.StuID] = row
	}
	for i := range tasks {
		if tasks[i].StudentBanned {
			continue
		}
		counts := countsByStudent[tasks[i].StuID]
		if counts.Eligible == 0 && counts.Total > 0 {
			tasks[i].ExecutionBlockedReason = "该学生的所有绑定用户账号均处于封禁状态，任务已暂停执行。解除任一绑定用户的封禁后可恢复。"
		} else if counts.Total == 0 {
			tasks[i].ExecutionBlockedReason = "该学生当前没有绑定用户，任务已暂停执行。恢复有效绑定后可继续运行。"
		}
	}
	return nil
}

func StudentBanMessage(ban StudentBan) string {
	period := formatBanPeriod(&ban.BannedAt, ban.ExpiresAt)
	message := fmt.Sprintf("该学生账号已被平台封禁（%s）", period)
	if ban.Reason != "" {
		message += "，原因：" + ban.Reason
	}
	return message + "。如有问题，请联系管理员。"
}

func formatBanPeriod(start, expiresAt *time.Time) string {
	if expiresAt == nil {
		return "永久"
	}
	until := expiresAt.Local().Format("2006-01-02 15:04")
	if start == nil {
		return "至 " + until
	}
	days := int(math.Round(expiresAt.Sub(*start).Hours() / 24))
	return fmt.Sprintf("%d 天，至 %s", days, until)
}
