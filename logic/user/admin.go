package user

import (
	"errors"
	"strings"
	"time"

	"dormcheck/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AdminUser struct {
	ID            int        `json:"id"`
	Username      string     `json:"username"`
	Email         string     `json:"email"`
	EmailVerified bool       `json:"email_verified"`
	Role          int        `json:"role"`
	Banned        bool       `json:"banned"`
	BanReason     string     `json:"ban_reason"`
	BannedAt      *time.Time `json:"banned_at"`
	BanExpiresAt  *time.Time `json:"ban_expires_at"`
}

type AdminUserPage struct {
	Users    []AdminUser `json:"users"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

func ListUsers(query string, page, pageSize int) (*AdminUserPage, error) {
	db := database.DB.Model(&database.User{})
	query = strings.TrimSpace(query)
	if query != "" {
		pattern := "%" + query + "%"
		db = db.Where("username ILIKE ? OR email ILIKE ?", pattern, pattern)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	var users []AdminUser
	if err := db.Select("id", "username", "email", "email_verified", "role", "banned", "ban_reason", "banned_at", "ban_expires_at").
		Order("id ASC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&users).Error; err != nil {
		return nil, err
	}
	for i := range users {
		users[i].Banned = database.IsUserBanActive(database.User{Banned: users[i].Banned, BanExpiresAt: users[i].BanExpiresAt}, time.Now())
	}
	return &AdminUserPage{Users: users, Total: total, Page: page, PageSize: pageSize}, nil
}

func BanUser(actorID, targetID, durationDays int, reason string) (*AdminUser, error) {
	if targetID <= 0 || targetID == actorID {
		return nil, errors.New("不能封禁当前账号或无效用户")
	}
	if durationDays < 0 || durationDays > 36500 {
		return nil, errors.New("封禁时长无效，天数应为 0 到 36500；0 表示永久")
	}
	if len([]rune(reason)) > 500 {
		return nil, errors.New("封禁原因不能超过 500 个字符")
	}
	now := time.Now()
	var expiresAt *time.Time
	if durationDays > 0 {
		value := now.AddDate(0, 0, durationDays)
		expiresAt = &value
	}
	var updated AdminUser
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var actor, target database.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&actor, actorID).Error; err != nil || !database.IsAdminRole(actor.Role) {
			return errors.New("需要管理员权限")
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&target, targetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("用户不存在")
			}
			return err
		}
		if actor.Role == database.RoleAdmin && target.Role == database.RoleSuperAdmin {
			return errors.New("管理员不能封禁超级管理员")
		}
		if err := tx.Model(&target).Updates(map[string]interface{}{
			"banned": true, "ban_reason": strings.TrimSpace(reason), "banned_at": now,
			"ban_expires_at": expiresAt, "banned_by": actorID,
		}).Error; err != nil {
			return err
		}
		updated = AdminUser{ID: target.ID, Username: target.Username, Email: target.Email, EmailVerified: target.EmailVerified, Role: target.Role, Banned: true, BanReason: strings.TrimSpace(reason), BannedAt: &now, BanExpiresAt: expiresAt}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func UnbanUser(actorID, targetID int) error {
	if targetID <= 0 || targetID == actorID {
		return errors.New("不能解除当前账号的封禁或用户 ID 无效")
	}
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var actor, target database.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&actor, actorID).Error; err != nil || !database.IsAdminRole(actor.Role) {
			return errors.New("需要管理员权限")
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&target, targetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("用户不存在")
			}
			return err
		}
		if actor.Role == database.RoleAdmin && target.Role == database.RoleSuperAdmin {
			return errors.New("管理员不能解除超级管理员的封禁")
		}
		return tx.Model(&target).Updates(map[string]interface{}{
			"banned": false, "ban_reason": "", "banned_at": nil, "ban_expires_at": nil, "banned_by": 0,
		}).Error
	})
}

func UpdateUserRole(actorID, targetID, nextRole int) (*AdminUser, error) {
	if targetID <= 0 {
		return nil, errors.New("用户 ID 无效")
	}
	if !isAssignableRole(nextRole) {
		return nil, errors.New("无效的用户角色")
	}

	var updated AdminUser
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var actor, target database.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&actor, actorID).Error; err != nil {
			return errors.New("当前管理员不存在")
		}
		if !database.IsAdminRole(actor.Role) {
			return errors.New("需要管理员权限")
		}
		if targetID == actorID {
			return errors.New("不能修改自己的角色")
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&target, targetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("用户不存在")
			}
			return err
		}
		if actor.Role == database.RoleAdmin && target.Role == database.RoleSuperAdmin {
			return errors.New("管理员不能修改超级管理员")
		}

		if err := tx.Model(&target).Updates(map[string]interface{}{
			"role":          nextRole,
			"token_version": gorm.Expr("token_version + 1"),
		}).Error; err != nil {
			return err
		}
		updated = AdminUser{
			ID:            target.ID,
			Username:      target.Username,
			Email:         target.Email,
			EmailVerified: target.EmailVerified,
			Role:          nextRole,
			Banned:        database.IsUserBanActive(target, time.Now()),
			BanReason:     target.BanReason,
			BannedAt:      target.BannedAt,
			BanExpiresAt:  target.BanExpiresAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func isAssignableRole(role int) bool {
	return role == database.RoleAdmin || role == database.RoleUser || role == database.RoleSponsor || role == database.RoleSuperAdmin
}
