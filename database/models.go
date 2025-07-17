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
	UserStudents  []UserStudent `gorm:"foreignKey:UserID"`
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
	UserID int    `gorm:"index"`
	StuID  string `gorm:"not null"`
	Name   string `gorm:""`
}

type Student struct { // 存储学生信息（学号、密码、cookies等）
	StuID     string    `gorm:"primaryKey"`
	Password  string    `gorm:"not null"`
	Cookies   string    `gorm:"type:text"` // 存储序列化后的 cookies
	LastLogin time.Time `gorm:"not null"`
	Name      string    `gorm:""`
}

type Task struct {
	ID           uint   `gorm:"primaryKey"`
	UserID       int    `gorm:"index;uniqueIndex:idx_user_activity_time"`
	StuID        string `gorm:"index;uniqueIndex:idx_user_activity_time"`
	ActivityID   string `gorm:"uniqueIndex:idx_user_activity_time"`
	Name         string // 学生姓名
	ActivityName string // 活动名称
	Address      string
	Longitude    float64
	Latitude     float64
	SignTime     string // 格式："HH:mm"

	NotifyEmail string `gorm:"size:255"` // ✅ 新增：用于通知的邮箱，可为空

	Enabled    bool
	ExecStatus string // "pending" | "success" | "failed"
	RetryCount int
	MaxRetry   int
	LastError  string
	ExecutedAt time.Time
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
