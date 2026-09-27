// database/models.go
package database

import "time"

type User struct { // 平台用户信息
	ID            int    `gorm:"primaryKey"`
	Username      string `gorm:"unique;not null"` // 用户名，唯一必填
	Email         string `gorm:"unique;not null"` // 邮箱，唯一必填
	EmailVerified bool   `gorm:"default:false"`   // 邮箱验证状态
	Password      string `gorm:"not null"`        // 密码哈希
	TokenVersion  int
	Role          int
	Banned        bool       `gorm:"not null;default:false;index"`
	BanReason     string     `gorm:"type:text"`
	BannedAt      *time.Time `gorm:"index"`
	BanExpiresAt  *time.Time `gorm:"index"`
	BannedBy      int
	UserStudents  []UserStudent `gorm:"foreignKey:UserID"`
}

type StudentBan struct {
	StuID     string     `gorm:"primaryKey;size:64" json:"stu_id"`
	Reason    string     `gorm:"type:text" json:"reason"`
	BannedAt  time.Time  `gorm:"not null;index" json:"banned_at"`
	ExpiresAt *time.Time `gorm:"index" json:"expires_at"`
	BannedBy  int        `gorm:"not null" json:"banned_by"`
}

type SystemSetting struct {
	Key       string `gorm:"primaryKey;size:128"`
	Value     string `gorm:"type:text;not null"`
	IsSecret  bool   `gorm:"not null;default:false"`
	UpdatedBy int    `gorm:"not null;default:0"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PlatformPost is a dated platform update or announcement.
type PlatformPost struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Kind        string     `gorm:"size:16;not null;index" json:"kind"`
	Title       string     `gorm:"size:120;not null" json:"title"`
	Body        string     `gorm:"type:text;not null" json:"body"`
	EventDate   time.Time  `gorm:"type:date;not null;index" json:"event_date"`
	Pinned      bool       `gorm:"not null;default:false" json:"pinned"`
	Published   bool       `gorm:"not null;default:false;index" json:"published"`
	PublishedAt *time.Time `json:"published_at"`
	AuthorID    int        `gorm:"not null" json:"-"`
	AuthorName  string     `gorm:"size:255;not null" json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type EmailVerificationCode struct {
	ID        int       `gorm:"primaryKey"`
	Email     string    `gorm:"index;not null"`
	Code      string    `gorm:"not null"` // 验证码，6位或其他长度
	Purpose   string    `gorm:"not null"` // 用途：register/login/reset
	ExpiresAt time.Time `gorm:"not null"` // 过期时间
	CreatedAt time.Time
}

type UserStudent struct { // 用户绑定学生
	ID     int    `gorm:"primaryKey"`
	UserID int    `gorm:"index;uniqueIndex:idx_user_student_binding"`
	StuID  string `gorm:"index;not null;uniqueIndex:idx_user_student_binding"`
	Name   string `gorm:""`
}

type Student struct { // 存储学生信息（学号、密码、cookies等）
	StuID            string     `gorm:"primaryKey"`
	Password         string     `gorm:"not null"`
	Cookies          string     `gorm:"type:text"` // 存储序列化后的 cookies
	LastLogin        time.Time  `gorm:"not null"`
	Name             string     `gorm:""`
	AuthStatus       string     `gorm:"size:16;not null;default:valid"`
	AuthFailedAt     *time.Time `gorm:"index"`
	AuthError        string     `gorm:"type:text"`
	AuthNoticeSentAt *time.Time
}

type Task struct {
	ID                uint   `gorm:"primaryKey"`
	UserID            int    `gorm:"index"` // 创建该任务的用户，仅用于兼容旧数据
	StuID             string `gorm:"index;uniqueIndex:idx_student_activity_task"`
	ActivityID        string `gorm:"uniqueIndex:idx_student_activity_task;index:idx_task_activity"`
	Name              string // 学生姓名
	ActivityName      string // 活动名称
	ActivityCollege   string
	SignMode          string
	ActivityTimeRange string
	ActivityStartDate string
	ActivityEndDate   string
	SignType          int
	QRCodeType        int
	Address           string
	Longitude         float64
	Latitude          float64
	SignTime          string `gorm:"index:idx_task_dispatch,priority:2"` // 格式："HH:mm"

	NotifyEmail string `gorm:"size:255"` // ✅ 新增：用于通知的邮箱，可为空

	Enabled          bool   `gorm:"index:idx_task_dispatch,priority:1"`
	ExecStatus       string `gorm:"index:idx_task_dispatch,priority:3"` // "pending" | "success" | "failed"
	RetryCount       int
	MaxRetry         int
	LastError        string
	ExecutedAt       time.Time  `gorm:"index"`
	RunningAt        *time.Time `gorm:"index"`
	LastManualAt     *time.Time
	LastManualStatus string
	LastManualError  string

	ActivityState          string `gorm:"size:32;not null;default:normal"`
	ActivityIssue          string `gorm:"type:text"`
	ActivityCheckedAt      *time.Time
	ActivityAutoPaused     bool
	ActivityOverride       bool
	AuthAutoPaused         bool
	StudentBanned          bool       `gorm:"-" json:"StudentBanned"`
	StudentBanReason       string     `gorm:"-" json:"StudentBanReason"`
	StudentBanExpiresAt    *time.Time `gorm:"-" json:"StudentBanExpiresAt"`
	ExecutionBlockedReason string     `gorm:"-" json:"ExecutionBlockedReason"`
	CancelAfterRun         bool
}

// 赞助激活码
type SponsorActivationCode struct {
	ID        uint   `gorm:"primaryKey"`
	Code      string `gorm:"unique;not null"` // 激活码字符串，唯一
	Used      bool   `gorm:"default:false"`   // 是否已被使用
	UsedBy    *int   // 使用者用户ID，空表示未用
	UsedAt    *time.Time
	CreatedAt time.Time
}
