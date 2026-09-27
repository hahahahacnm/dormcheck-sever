package scheduler

import (
	"dormcheck/config"
	"dormcheck/database"
	"dormcheck/logic/student"
	"dormcheck/utils"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StartCookieRefresher refreshes each unique student account once daily at the configured time.
func StartCookieRefresher() {
	go refreshStudentCookies()
	startConfiguredDailyJob("cookie_refresh_time", "18:00", false, refreshStudentCookies)
}

var refreshMu sync.Mutex

func refreshStudentCookies() {
	if !refreshMu.TryLock() {
		return
	}
	defer refreshMu.Unlock()
	log.Println("🔄 正在刷新所有学生 cookies...")

	var students []database.Student
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if err := database.DB.Where("stu_id IN (SELECT stu_id FROM user_students) AND last_login < ?", dayStart).Find(&students).Error; err != nil {
		log.Printf("❌ 查询学生失败: %v", err)
		return
	}

	if len(students) == 0 {
		return
	}
	workers := config.GetInt("cookie_refresh_concurrency", 3)
	if workers < 1 {
		workers = 1
	}
	if workers > 20 {
		workers = 20
	}
	jobs := make(chan database.Student)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for stu := range jobs {
				refreshOneStudent(stu)
			}
		}()
	}
	interval := time.Hour / time.Duration(len(students))
	for i, stu := range students {
		jobs <- stu
		if i < len(students)-1 {
			time.Sleep(interval)
		}
	}
	close(jobs)
	wg.Wait()
	log.Println("✅ 所有学生 cookies 刷新完成")
}

func refreshOneStudent(stu database.Student) {
	cookies, err := student.LoginWithoutBind(stu.StuID, stu.Password)

	// ===== 登录失败或 cookies 为空 =====
	if err != nil || cookies == nil || len(cookies) == 0 {
		failureMessage := "微学工登录未返回 Cookie"
		if err != nil {
			failureMessage = err.Error()
		}
		log.Printf("⚠️ 登录失败: 学号=%s，错误=%s", stu.StuID, failureMessage)

		// 网络、验证码或 AI 暂时故障不标成密码失效；仅凭证类失败累计失效时长。
		if isStudentCredentialFailure(failureMessage) {
			shouldNotify, locked := recordStudentCredentialFailure(stu, failureMessage)
			if locked {
				lockTasksForInvalidStudent(stu.StuID)
			}
			if shouldNotify {
				sendStudentCredentialFailureNotice(stu, failureMessage)
			}
		}

		// 登录失败 → 不更新 cookies，保留旧值
		return
	}

	// ===== 登录成功才更新 cookies =====
	stu.Cookies = utils.SerializeCookies(cookies)
	stu.AuthStatus = "valid"
	stu.AuthFailedAt = nil
	stu.AuthError = ""
	stu.AuthNoticeSentAt = nil
	result := database.DB.Model(&database.Student{}).
		Where("stu_id = ? AND last_login <= ?", stu.StuID, stu.LastLogin).
		Updates(map[string]interface{}{
			"cookies": stu.Cookies, "last_login": time.Now(), "auth_status": "valid",
			"auth_failed_at": nil, "auth_error": "", "auth_notice_sent_at": nil,
		})
	if result.Error != nil {
		log.Printf("❌ 保存失败: 学号=%s, 错误=%v", stu.StuID, result.Error)
	} else if result.RowsAffected > 0 {
		log.Printf("✅ 学号 %s cookies 已更新", stu.StuID)
		restoreTasksAfterStudentReverification(stu.StuID)
	} else {
		log.Printf("跳过过期的 cookies 刷新结果: 学号=%s", stu.StuID)
	}

}

func isStudentCredentialFailure(message string) bool {
	message = strings.ToLower(message)
	for _, keyword := range []string{
		"用户名或密码错误", "用户名或者密码错误", "账号或密码错误", "账户或密码错误", "用户不存在或密码错误",
		"密码错误", "密码不正确", "密码已过期", "密码过期", "密码过于简单", "账号不存在", "账户不存在", "用户不存在",
		"账号已锁定", "账户已锁定", "incorrect password", "invalid password", "incorrect username or password", "user does not exist",
	} {
		if strings.Contains(message, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// Record a credential failure once per student. Authentication mail is capped
// at once per local calendar day and stops when the continuous failure reaches
// seven days. The row lock makes duplicate refresh workers idempotent.
func recordStudentCredentialFailure(stu database.Student, failureMessage string) (shouldNotify, locked bool) {
	now := time.Now()
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var current database.Student
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "stu_id = ?", stu.StuID).Error; err != nil {
			return err
		}
		// Ignore an old refresh result if the user has since reverified.
		if current.LastLogin.After(stu.LastLogin) {
			return nil
		}
		failedAt := current.AuthFailedAt
		if current.AuthStatus != "invalid" && current.AuthStatus != "locked" || failedAt == nil {
			failedAt = &now
			current.AuthFailedAt = failedAt
		}
		current.AuthError = failureMessage
		shouldNotify, locked = credentialFailurePolicy(now, *failedAt, current.AuthNoticeSentAt)
		if locked {
			current.AuthStatus = "locked"
		} else {
			current.AuthStatus = "invalid"
			if shouldNotify {
				current.AuthNoticeSentAt = &now
			}
		}
		return tx.Save(&current).Error
	})
	if err != nil {
		log.Printf("保存学生账号失效状态失败: 学号=%s，错误=%v", stu.StuID, err)
		return false, false
	}
	return shouldNotify, locked
}

func credentialFailurePolicy(now, failedAt time.Time, lastNotice *time.Time) (notify, lock bool) {
	if now.Sub(failedAt) >= 7*24*time.Hour {
		return false, true
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return lastNotice == nil || lastNotice.Before(dayStart), false
}

func sendStudentCredentialFailureNotice(stu database.Student, failureMessage string) {
	var bindings []database.UserStudent
	if err := database.DB.Where("stu_id = ?", stu.StuID).Find(&bindings).Error; err != nil {
		log.Printf("查询失效学生绑定关系失败: 学号=%s，错误=%v", stu.StuID, err)
		return
	}
	seen := make(map[int]struct{}, len(bindings))
	for _, binding := range bindings {
		if _, exists := seen[binding.UserID]; exists {
			continue
		}
		seen[binding.UserID] = struct{}{}
		var account database.User
		if err := database.DB.First(&account, binding.UserID).Error; err != nil {
			log.Printf("查询失效学生所属用户失败: UserID=%d，错误=%v", binding.UserID, err)
			continue
		}
		if err := utils.SendAccountErrorEmail(account.Email, stu.Name, stu.StuID, failureMessage, time.Now()); err != nil {
			log.Printf("学生账号失效邮件发送失败: UserID=%d，错误=%v", account.ID, err)
		} else {
			log.Printf("已发送学生账号失效提醒: UserID=%d，学号=%s", account.ID, stu.StuID)
		}
	}
}

// Lock tasks only after a continuous seven-day credential failure.
func StartInvalidAccountTaskDisabler() {
	go func() {
		disableExpiredInvalidAccountTasks()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			disableExpiredInvalidAccountTasks()
		}
	}()
}

func disableExpiredInvalidAccountTasks() {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	var students []database.Student
	if err := database.DB.Where("auth_status = ? AND auth_failed_at IS NOT NULL AND auth_failed_at <= ?", "invalid", cutoff).Find(&students).Error; err != nil {
		log.Printf("查询超期失效账号失败: %v", err)
		return
	}
	for _, stu := range students {
		result := database.DB.Model(&database.Student{}).
			Where("stu_id = ? AND auth_status = ? AND auth_failed_at <= ?", stu.StuID, "invalid", cutoff).
			Update("auth_status", "locked")
		if result.Error != nil {
			log.Printf("锁定连续失效学生账号失败: %s: %v", stu.StuID, result.Error)
			continue
		}
		if result.RowsAffected == 0 {
			continue
		}
		lockTasksForInvalidStudent(stu.StuID)
	}
}

func restoreTasksAfterStudentReverification(stuID string) {
	result := database.DB.Model(&database.Task{}).
		Where("stu_id = ? AND auth_auto_paused = TRUE AND activity_state = ?", stuID, "normal").
		Updates(map[string]interface{}{"enabled": true, "auth_auto_paused": false})
	if result.Error != nil {
		log.Printf("学生重新验证后恢复任务失败: %s: %v", stuID, result.Error)
	} else if result.RowsAffected > 0 {
		log.Printf("学生重新验证成功，已恢复学号 %s 的 %d 个任务", stuID, result.RowsAffected)
	}
}

func lockTasksForInvalidStudent(stuID string) {
	result := database.DB.Model(&database.Task{}).
		Where("stu_id = ? AND enabled = TRUE", stuID).
		Updates(map[string]interface{}{"enabled": false, "auth_auto_paused": true})
	if result.Error != nil {
		log.Printf("锁定失效学生任务失败: %s: %v", stuID, result.Error)
		return
	}
	if result.RowsAffected > 0 {
		log.Printf("已锁定学号 %s 的 %d 个任务", stuID, result.RowsAffected)
	}
}
