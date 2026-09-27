package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"

	"dormcheck/database"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

var (
	settingsMu sync.RWMutex
	settings   = map[string]string{}
)

// InitConfig loads local environment variables used to connect to PostgreSQL
// and to seed settings during the first database initialization.
func InitConfig() {
	_ = godotenv.Load()
}

// LoadSettings creates missing defaults once, then loads the live settings cache.
func LoadSettings() error {
	generatedJWTSecret, err := generateSecret()
	if err != nil {
		return err
	}
	defaults := map[string]string{
		"jwt_secret":                     envOr("JWT_SECRET", generatedJWTSecret),
		"jwt_expiration_hours":           "720",
		"captcha_ai_api_key":             envOr("CAPTCHA_AI_API_KEY", os.Getenv("DASHSCOPE_API_KEY")),
		"captcha_ai_base_url":            "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
		"captcha_ai_model":               "qwen-vl-ocr-latest",
		"captcha_ai_timeout_seconds":     "30",
		"captcha_ai_attempts":            "3",
		"captcha_ai_system_prompt":       "你被使用api调用，作用是验证码识别.",
		"captcha_ai_user_prompt":         "4位长度字符类型验证码图像识别，只输出识别结果",
		"school_captcha_timeout_seconds": "10",
		"school_request_timeout_seconds": "30",
		"amap_web_key":                   os.Getenv("AMAP_WEB_KEY"),
		"amap_security_js_code":          os.Getenv("AMAP_SECURITY_JS_CODE"),
		"student_limit_user":             "2",
		"student_limit_sponsor":          "12",
		"student_limit_admin":            "0",
		"student_limit_super_admin":      "0",
		"smtp_host":                      envOr("SMTP_HOST", "smtp.exmail.qq.com"),
		"smtp_port":                      envOr("SMTP_PORT", "465"),
		"smtp_username":                  os.Getenv("SMTP_USER"),
		"smtp_password":                  os.Getenv("SMTP_PASSWORD"),
		"smtp_from_name":                 envOr("SMTP_FROM_NAME", "DormCheck 系统"),
		"smtp_ssl":                       envOr("SMTP_SSL", "true"),
		"username_min_length":            "3",
		"password_min_length":            "6",
		"email_code_ttl_minutes":         "15",
		"email_code_resend_seconds":      "60",
		"task_max_retries":               "3",
		"task_batch_size":                "10",
		"cookie_refresh_time":            "18:00",
		"cookie_refresh_concurrency":     "3",
		"activity_audit_time":            "04:00",
	}

	for key, value := range defaults {
		var setting database.SystemSetting
		err := database.DB.First(&setting, "key = ?", key).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			setting = database.SystemSetting{Key: key, Value: value, IsSecret: isSecret(key)}
			if err := database.DB.Create(&setting).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if key == "jwt_secret" && setting.Value == "" {
			if err := database.DB.Model(&setting).Update("value", value).Error; err != nil {
				return err
			}
		}
	}
	return ReloadSettings()
}

func ReloadSettings() error {
	var rows []database.SystemSetting
	if err := database.DB.Find(&rows).Error; err != nil {
		return err
	}
	loaded := make(map[string]string, len(rows))
	for _, row := range rows {
		loaded[row.Key] = row.Value
	}
	if strings.TrimSpace(loaded["jwt_secret"]) == "" {
		return errors.New("JWT signing secret is not configured")
	}
	settingsMu.Lock()
	settings = loaded
	settingsMu.Unlock()
	return nil
}

func Get(key string) string {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return settings[key]
}

func GetInt(key string, fallback int) int {
	value, err := strconv.Atoi(Get(key))
	if err != nil {
		return fallback
	}
	return value
}

func GetBool(key string, fallback bool) bool {
	value, err := strconv.ParseBool(Get(key))
	if err != nil {
		return fallback
	}
	return value
}

func JWTSecret() []byte { return []byte(Get("jwt_secret")) }

func isSecret(key string) bool {
	switch key {
	case "jwt_secret", "captcha_ai_api_key", "amap_web_key", "amap_security_js_code", "smtp_username", "smtp_password":
		return true
	default:
		return false
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func generateSecret() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(secret), nil
}
