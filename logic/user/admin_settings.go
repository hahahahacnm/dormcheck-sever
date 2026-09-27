package user

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dormcheck/config"
	"dormcheck/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AdminSettingsSnapshot struct {
	Values  map[string]string `json:"values"`
	Secrets map[string]string `json:"secrets"`
}

var settingKeys = map[string]bool{
	"jwt_expiration_hours":           false,
	"captcha_ai_base_url":            false,
	"captcha_ai_model":               false,
	"captcha_ai_timeout_seconds":     false,
	"captcha_ai_attempts":            false,
	"captcha_ai_system_prompt":       false,
	"captcha_ai_user_prompt":         false,
	"school_captcha_timeout_seconds": false,
	"school_request_timeout_seconds": false,
	"smtp_host":                      false,
	"smtp_port":                      false,
	"smtp_from_name":                 false,
	"smtp_ssl":                       false,
	"student_limit_user":             false,
	"student_limit_sponsor":          false,
	"student_limit_admin":            false,
	"student_limit_super_admin":      false,
	"username_min_length":            false,
	"password_min_length":            false,
	"email_code_ttl_minutes":         false,
	"email_code_resend_seconds":      false,
	"task_max_retries":               false,
	"task_batch_size":                false,
	"cookie_refresh_time":            false,
	"cookie_refresh_concurrency":     false,
	"activity_audit_time":            false,
	"jwt_secret":                     true,
	"captcha_ai_api_key":             true,
	"amap_web_key":                   true,
	"amap_security_js_code":          true,
	"smtp_username":                  true,
	"smtp_password":                  true,
}

func GetAdminSettings(actorID int) (*AdminSettingsSnapshot, error) {
	if err := requireSuperAdmin(actorID); err != nil {
		return nil, err
	}
	var rows []database.SystemSetting
	if err := database.DB.Order("key ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	snapshot := &AdminSettingsSnapshot{Values: map[string]string{}, Secrets: map[string]string{}}
	for _, row := range rows {
		secret, managed := settingKeys[row.Key]
		if !managed {
			continue
		}
		if secret {
			snapshot.Secrets[row.Key] = row.Value
		} else {
			snapshot.Values[row.Key] = row.Value
		}
	}
	return snapshot, nil
}

func UpdateAdminSettings(actorID int, values, secrets map[string]string, clearSecrets []string) error {
	if err := requireSuperAdmin(actorID); err != nil {
		return err
	}
	for key, value := range values {
		secret, managed := settingKeys[key]
		if !managed || secret {
			return fmt.Errorf("不允许修改设置项 %q", key)
		}
		if err := validateSetting(key, value); err != nil {
			return err
		}
		values[key] = strings.TrimSpace(value)
	}
	for key, value := range secrets {
		secret, managed := settingKeys[key]
		if !managed || !secret {
			return fmt.Errorf("不允许修改密钥项 %q", key)
		}
		if key == "jwt_secret" && strings.TrimSpace(value) == "" {
			return errors.New("JWT 密钥不能为空")
		}
		if len(value) > 4096 {
			return fmt.Errorf("设置项 %q 超过允许长度", key)
		}
	}
	for _, key := range clearSecrets {
		secret, managed := settingKeys[key]
		if !managed || !secret || key == "jwt_secret" {
			return fmt.Errorf("不允许清除此密钥项 %q", key)
		}
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		for key, value := range values {
			if err := upsertSetting(tx, key, value, false, actorID); err != nil {
				return err
			}
		}
		if retries, ok := values["task_max_retries"]; ok {
			count, _ := strconv.Atoi(retries)
			if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Model(&database.Task{}).Update("max_retry", count+1).Error; err != nil {
				return err
			}
		}
		for key, value := range secrets {
			if strings.TrimSpace(value) == "" {
				continue
			}
			if err := upsertSetting(tx, key, value, true, actorID); err != nil {
				return err
			}
		}
		for _, key := range clearSecrets {
			if err := upsertSetting(tx, key, "", true, actorID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return config.ReloadSettings()
}

func requireSuperAdmin(userID int) error {
	var actor database.User
	if err := database.DB.Select("id", "role").First(&actor, userID).Error; err != nil {
		return errors.New("当前用户不存在")
	}
	if actor.Role != database.RoleSuperAdmin {
		return errors.New("需要超级管理员权限")
	}
	return nil
}

func upsertSetting(tx *gorm.DB, key, value string, secret bool, actorID int) error {
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "is_secret", "updated_by", "updated_at"}),
	}).Create(&database.SystemSetting{
		Key: key, Value: value, IsSecret: secret, UpdatedBy: actorID,
	}).Error
}

func validateSetting(key, value string) error {
	value = strings.TrimSpace(value)
	if len(value) > 4000 {
		return fmt.Errorf("设置项 %q 超过允许长度", key)
	}
	switch key {
	case "captcha_ai_base_url":
		parsed, err := url.ParseRequestURI(value)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return errors.New("AI 接口地址必须是有效的 HTTP 或 HTTPS URL")
		}
	case "captcha_ai_model", "captcha_ai_system_prompt", "captcha_ai_user_prompt", "smtp_host", "smtp_from_name":
		if value == "" {
			return fmt.Errorf("设置项 %q 不能为空", key)
		}
	case "cookie_refresh_time", "activity_audit_time":
		if _, err := time.Parse("15:04", value); err != nil {
			return fmt.Errorf("设置项 %q 必须是有效的 24 小时时间（HH:mm）", key)
		}
	case "smtp_ssl":
		if _, err := strconv.ParseBool(value); err != nil {
			return errors.New("SMTP SSL 必须是 true 或 false")
		}
	default:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("设置项 %q 必须是整数", key)
		}
		min, max := 0, 10000
		switch key {
		case "jwt_expiration_hours":
			min, max = 1, 8760
		case "captcha_ai_timeout_seconds":
			min, max = 1, 300
		case "school_captcha_timeout_seconds":
			min, max = 1, 120
		case "school_request_timeout_seconds":
			min, max = 5, 120
		case "captcha_ai_attempts":
			min, max = 1, 10
		case "smtp_port":
			min, max = 1, 65535
		case "username_min_length":
			min, max = 3, 64
		case "password_min_length":
			min, max = 6, 128
		case "email_code_ttl_minutes":
			min, max = 1, 1440
		case "email_code_resend_seconds":
			max = 86400
		case "student_limit_user", "student_limit_sponsor", "student_limit_admin", "student_limit_super_admin":
			max = 10000
		case "task_max_retries":
			max = 20
		case "task_batch_size":
			min, max = 1, 100
		case "cookie_refresh_concurrency":
			min, max = 1, 20
		default:
			return fmt.Errorf("未知设置项 %q", key)
		}
		if parsed < min || parsed > max {
			return fmt.Errorf("设置项 %q 的值必须在 %d 到 %d 之间", key, min, max)
		}
	}
	return nil
}
