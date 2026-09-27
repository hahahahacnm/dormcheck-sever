package user

import (
	"strings"
	"time"

	"dormcheck/database"
)

type AdminStudentView struct {
	StuID               string     `json:"stu_id"`
	Name                string     `json:"name"`
	Password            string     `json:"password"`
	LastLogin           time.Time  `json:"last_login"`
	AccountStatus       string     `json:"account_status"`
	AuthFailedAt        *time.Time `json:"auth_failed_at"`
	AuthError           string     `json:"auth_error"`
	StudentBanned       bool       `json:"student_banned"`
	StudentBanReason    string     `json:"student_ban_reason"`
	StudentBanExpiresAt *time.Time `json:"student_ban_expires_at"`
	BindingCount        int64      `json:"binding_count"`
	TaskCount           int64      `json:"task_count"`
	BoundUsers          []string   `json:"bound_users" gorm:"-"`
}

func ListStudents(query string, page, pageSize int) ([]AdminStudentView, int64, error) {
	db := database.DB.Table("students s")
	query = strings.TrimSpace(query)
	if query != "" {
		like := "%" + query + "%"
		db = db.Where(`(s.stu_id ILIKE ? OR s.name ILIKE ? OR EXISTS (
			SELECT 1 FROM user_students us JOIN users u ON u.id = us.user_id
			WHERE us.stu_id = s.stu_id AND (u.username ILIKE ? OR u.email ILIKE ?)))`, like, like, like, like)
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
	if pageSize > 100 {
		pageSize = 100
	}
	type row struct {
		AdminStudentView
		BanStuID *string `gorm:"column:ban_stu_id"`
	}
	var rows []row
	selectSQL := `s.stu_id, s.name, s.password, s.last_login,
		COALESCE(s.auth_status, 'valid') AS account_status, s.auth_failed_at, s.auth_error,
		(student_bans.stu_id IS NOT NULL) AS student_banned,
		COALESCE(student_bans.reason, '') AS student_ban_reason,
		student_bans.expires_at AS student_ban_expires_at,
		student_bans.stu_id AS ban_stu_id,
		(SELECT COUNT(*) FROM user_students us WHERE us.stu_id = s.stu_id) AS binding_count,
		(SELECT COUNT(*) FROM tasks t WHERE t.stu_id = s.stu_id) AS task_count`
	if err := db.Select(selectSQL).
		Joins("LEFT JOIN student_bans ON student_bans.stu_id = s.stu_id AND (student_bans.expires_at IS NULL OR student_bans.expires_at > ?)", time.Now()).
		Order("s.stu_id ASC").Limit(pageSize).Offset((page - 1) * pageSize).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	students := make([]AdminStudentView, len(rows))
	ids := make([]string, 0, len(rows))
	for i, item := range rows {
		students[i] = item.AdminStudentView
		students[i].StudentBanned = item.BanStuID != nil
		students[i].BoundUsers = []string{}
		ids = append(ids, item.StuID)
	}
	if len(ids) == 0 {
		return students, total, nil
	}
	var owners []struct {
		StuID    string
		Username string
		Email    string
	}
	if err := database.DB.Table("user_students").Select("user_students.stu_id, users.username, users.email").
		Joins("JOIN users ON users.id = user_students.user_id").Where("user_students.stu_id IN ?", ids).
		Order("users.username ASC").Find(&owners).Error; err != nil {
		return nil, 0, err
	}
	index := make(map[string]int, len(students))
	for i := range students {
		index[students[i].StuID] = i
	}
	for _, owner := range owners {
		if i, ok := index[owner.StuID]; ok {
			students[i].BoundUsers = append(students[i].BoundUsers, owner.Username+" · "+owner.Email)
		}
	}
	return students, total, nil
}
