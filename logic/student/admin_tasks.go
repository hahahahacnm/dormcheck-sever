package student

import (
	"errors"
	"strings"
	"time"

	"dormcheck/database"
)

type ActivityTaskSummary struct {
	ActivityID        string `json:"activity_id"`
	ActivityName      string `json:"activity_name"`
	College           string `json:"college"`
	SignMode          string `json:"sign_mode"`
	TimeRange         string `json:"time_range"`
	StartDate         string `json:"start_date"`
	EndDate           string `json:"end_date"`
	SignType          int    `json:"sign_type"`
	QRCodeType        int    `json:"qrcode_type"`
	TaskCount         int    `json:"task_count"`
	UserCount         int    `json:"user_count"`
	RunningCount      int    `json:"running_count"`
	PausedCount       int    `json:"paused_count"`
	RecentFailCount   int    `json:"recent_fail_count"`
	IdentityFailCount int    `json:"identity_fail_count"`
}

type AdminTaskView struct {
	database.Task
	BoundUsers []BoundTaskUser `json:"bound_users" gorm:"-"`
}

type BoundTaskUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

func GetActivityTaskSummaries() ([]ActivityTaskSummary, error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	now := time.Now()
	var summaries []ActivityTaskSummary
	err := database.DB.Raw(`
		SELECT activity_id, MAX(activity_name) AS activity_name,
		       MAX(activity_college) AS college, MAX(sign_mode) AS sign_mode,
		       MAX(activity_time_range) AS time_range,
		       MAX(activity_start_date) AS start_date, MAX(activity_end_date) AS end_date,
	       MAX(sign_type) AS sign_type, MAX(qr_code_type) AS qr_code_type,
	       COUNT(DISTINCT tasks.id) AS task_count, COUNT(DISTINCT user_students.user_id) AS user_count,
	       COUNT(DISTINCT tasks.id) FILTER (WHERE enabled = TRUE AND exec_status <> 'success' AND NOT EXISTS (
	          SELECT 1 FROM student_bans sb WHERE sb.stu_id = tasks.stu_id AND (sb.expires_at IS NULL OR sb.expires_at > ?)) AND EXISTS (
	          SELECT 1 FROM user_students us JOIN users u ON u.id = us.user_id WHERE us.stu_id = tasks.stu_id AND
	          (u.banned = FALSE OR (u.ban_expires_at IS NOT NULL AND u.ban_expires_at <= ?)))) AS running_count,
	       COUNT(DISTINCT tasks.id) FILTER (WHERE enabled = FALSE OR EXISTS (
	          SELECT 1 FROM student_bans sb WHERE sb.stu_id = tasks.stu_id AND (sb.expires_at IS NULL OR sb.expires_at > ?)) OR NOT EXISTS (
	          SELECT 1 FROM user_students us JOIN users u ON u.id = us.user_id WHERE us.stu_id = tasks.stu_id AND
	          (u.banned = FALSE OR (u.ban_expires_at IS NOT NULL AND u.ban_expires_at <= ?)))) AS paused_count,
	       COUNT(DISTINCT tasks.id) FILTER (WHERE exec_status = 'failed' AND executed_at >= ?) AS recent_fail_count,
	       COUNT(DISTINCT tasks.id) FILTER (WHERE exec_status = 'failed' AND executed_at >= ? AND
	          (LOWER(last_error) LIKE '%cookie%' OR LOWER(last_error) LIKE '%session%' OR
	           LOWER(last_error) LIKE '%token%' OR last_error LIKE '%登录%' OR
	           last_error LIKE '%身份%' OR last_error LIKE '%认证%' OR last_error LIKE '%过期%')) AS identity_fail_count
		FROM tasks
		LEFT JOIN user_students ON user_students.stu_id = tasks.stu_id
		GROUP BY activity_id
		ORDER BY activity_name ASC, activity_id ASC`, now, now, now, now, cutoff, cutoff).Scan(&summaries).Error
	if err != nil {
		return nil, err
	}
	return summaries, nil
}

func GetAdminTasksByActivity(activityID string) ([]AdminTaskView, error) {
	var tasks []AdminTaskView
	err := database.DB.Table("tasks").
		Select("tasks.*").
		Where("tasks.activity_id = ?", activityID).
		Order("tasks.user_id ASC, tasks.stu_id ASC").
		Find(&tasks).Error
	if err != nil || len(tasks) == 0 {
		return tasks, err
	}
	if err := attachBoundUsers(tasks); err != nil {
		return nil, err
	}
	if err := attachStudentBanDetails(tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func GetAdminTaskList(query, activityID, status string, page, pageSize int) ([]AdminTaskView, int64, error) {
	db := database.DB.Table("tasks")
	if activityID != "" {
		db = db.Where("tasks.activity_id = ?", activityID)
	}
	switch status {
	case "running":
		db = db.Where(`tasks.enabled = TRUE AND tasks.exec_status <> ? AND NOT EXISTS (
			SELECT 1 FROM student_bans sb WHERE sb.stu_id = tasks.stu_id AND (sb.expires_at IS NULL OR sb.expires_at > ?)
		) AND EXISTS (SELECT 1 FROM user_students us JOIN users u ON u.id = us.user_id WHERE us.stu_id = tasks.stu_id AND
			(u.banned = FALSE OR (u.ban_expires_at IS NOT NULL AND u.ban_expires_at <= ?)))`, "success", time.Now(), time.Now())
	case "paused":
		db = db.Where(`tasks.enabled = FALSE OR EXISTS (
			SELECT 1 FROM student_bans sb WHERE sb.stu_id = tasks.stu_id AND (sb.expires_at IS NULL OR sb.expires_at > ?)
		) OR NOT EXISTS (SELECT 1 FROM user_students us JOIN users u ON u.id = us.user_id WHERE us.stu_id = tasks.stu_id AND
			(u.banned = FALSE OR (u.ban_expires_at IS NOT NULL AND u.ban_expires_at <= ?)))`, time.Now(), time.Now())
	case "success", "failed":
		db = db.Where("tasks.exec_status = ?", status)
	}
	if query = strings.TrimSpace(query); query != "" {
		like := "%" + strings.ToLower(query) + "%"
		db = db.Where(`(LOWER(tasks.stu_id) LIKE ? OR LOWER(tasks.name) LIKE ? OR
			LOWER(tasks.activity_name) LIKE ? OR EXISTS (
				SELECT 1 FROM user_students us JOIN users u ON u.id = us.user_id
				WHERE us.stu_id = tasks.stu_id AND
				(LOWER(u.username) LIKE ? OR LOWER(u.email) LIKE ?)))`, like, like, like, like, like)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	var tasks []AdminTaskView
	err := db.Select("tasks.*").Order("tasks.enabled DESC, tasks.executed_at DESC NULLS LAST, tasks.id DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).Find(&tasks).Error
	if err != nil || len(tasks) == 0 {
		return tasks, total, err
	}
	if err := attachBoundUsers(tasks); err != nil {
		return nil, 0, err
	}
	if err := attachStudentBanDetails(tasks); err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func attachStudentBanDetails(tasks []AdminTaskView) error {
	plain := make([]database.Task, len(tasks))
	for i := range tasks {
		plain[i] = tasks[i].Task
	}
	if err := database.AttachActiveStudentBans(plain, time.Now()); err != nil {
		return err
	}
	for i := range tasks {
		tasks[i].Task = plain[i]
	}
	return nil
}

func attachBoundUsers(tasks []AdminTaskView) error {
	stuIDs := make([]string, 0, len(tasks))
	seen := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		if _, exists := seen[task.StuID]; !exists {
			stuIDs = append(stuIDs, task.StuID)
			seen[task.StuID] = struct{}{}
		}
	}
	var bindings []struct {
		StuID string
		BoundTaskUser
	}
	err := database.DB.Table("user_students").
		Select("user_students.stu_id, users.id, users.username, users.email").
		Joins("JOIN users ON users.id = user_students.user_id").
		Where("user_students.stu_id IN ?", stuIDs).Find(&bindings).Error
	if err != nil {
		return err
	}
	usersByStudent := make(map[string][]BoundTaskUser)
	for _, binding := range bindings {
		usersByStudent[binding.StuID] = append(usersByStudent[binding.StuID], binding.BoundTaskUser)
	}
	for i := range tasks {
		tasks[i].BoundUsers = usersByStudent[tasks[i].StuID]
	}
	return nil
}

func RunTaskNow(taskID uint, actorID int, admin bool) (*database.Task, error) {
	var task database.Task
	if err := database.DB.First(&task, taskID).Error; err != nil {
		return nil, err
	}
	if !admin {
		var actor database.User
		if err := database.DB.First(&actor, actorID).Error; err != nil {
			return nil, err
		}
		if database.IsUserBanActive(actor, time.Now()) {
			return nil, errors.New(database.UserBanMessage(actor))
		}
		bound, err := UserHasStudentBinding(actorID, task.StuID)
		if err != nil {
			return nil, err
		}
		if !bound {
			return nil, errTaskPermissionDenied
		}
	}
	if err := EnsureStudentHasEligibleBinding(task.StuID); err != nil {
		return nil, err
	}
	if err := EnsureStudentNotBanned(task.StuID); err != nil {
		return nil, err
	}
	if err := ExecuteSignTaskNow(&task); errors.Is(err, ErrManualRunOutsideWindow) || errors.Is(err, ErrTaskAlreadyRunning) || errors.Is(err, ErrTaskResultNotSaved) {
		return nil, err
	}
	if err := database.DB.First(&task, taskID).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func EnsureStudentHasEligibleBinding(stuID string) error {
	var count int64
	err := database.DB.Table("user_students").Joins("JOIN users ON users.id = user_students.user_id").
		Where("user_students.stu_id = ? AND (users.banned = FALSE OR (users.ban_expires_at IS NOT NULL AND users.ban_expires_at <= ?))", stuID, time.Now()).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("该学生的所有绑定用户账号均处于封禁状态，任务暂停执行")
	}
	return nil
}

var errTaskPermissionDenied = taskPermissionError{}

type taskPermissionError struct{}

func (taskPermissionError) Error() string { return "permission denied" }
