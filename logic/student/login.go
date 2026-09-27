// logic/student/login.go
package student

import (
	"dormcheck/config"
	"dormcheck/database"
	"dormcheck/external/schoollogin"
	"dormcheck/utils"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LoginFailWithG 封装登录失败信息和 g 值
type LoginFailWithG struct {
	Err error  // 原始错误信息
	G   string // 初始密码修改标识
}

func (e *LoginFailWithG) Error() string {
	return e.Err.Error()
}

// LoginAndBindStudent 尝试登录微学工平台，并保存学生信息 + 用户绑定 + 姓名
func LoginAndBindStudent(userID int, stuID, plainPassword string) error {
	if err := EnsureStudentNotBanned(stuID); err != nil {
		return err
	}
	db := database.DB

	// 判断绑定数量限制
	var user database.User
	if err := db.First(&user, userID).Error; err != nil {
		return fmt.Errorf("用户不存在")
	}

	var currentCount int64
	if err := db.Model(&database.UserStudent{}).Where("user_id = ?", userID).Count(&currentCount).Error; err != nil {
		return fmt.Errorf("查询已绑定学生失败: %v", err)
	}

	var existingBinding int64
	if err := db.Model(&database.UserStudent{}).Where("user_id = ? AND stu_id = ?", userID, stuID).Count(&existingBinding).Error; err != nil {
		return fmt.Errorf("检查学生绑定关系失败: %v", err)
	}

	if existingBinding == 0 {
		switch user.Role {
		case database.RoleUser:
			limit := config.GetInt("student_limit_user", 2)
			if limit > 0 && currentCount >= int64(limit) {
				return fmt.Errorf("普通用户最多只能绑定 %d 名学生", limit)
			}
		case database.RoleSponsor:
			limit := config.GetInt("student_limit_sponsor", 12)
			if limit > 0 && currentCount >= int64(limit) {
				return fmt.Errorf("赞助用户最多只能绑定 %d 名学生", limit)
			}
		case database.RoleAdmin, database.RoleSuperAdmin:
			limitKey := "student_limit_admin"
			label := "管理员"
			if user.Role == database.RoleSuperAdmin {
				limitKey = "student_limit_super_admin"
				label = "超级管理员"
			}
			limit := config.GetInt(limitKey, 0)
			if limit > 0 && currentCount >= int64(limit) {
				return fmt.Errorf("%s最多只能绑定 %d 名学生", label, limit)
			}
		default:
			return fmt.Errorf("未知用户角色")
		}
	}

	studentName, err := VerifyAndSaveStudentCredentials(stuID, plainPassword)
	if err != nil {
		return err
	}
	if err := bindVerifiedStudent(userID, stuID, studentName); err != nil {
		return fmt.Errorf("用户与学号绑定失败: %v", err)
	}
	return nil
}

// VerifyAndSaveStudentCredentials refreshes the upstream session and persists
// the new credential/cookie pair without changing who owns the binding.
func VerifyAndSaveStudentCredentials(stuID, plainPassword string) (string, error) {
	if strings.TrimSpace(stuID) == "" || plainPassword == "" {
		return "", errors.New("学号和密码不能为空")
	}
	var lastErr error
	for i := 1; i <= config.GetInt("captcha_ai_attempts", 3); i++ {
		log.Printf("🔁 正在进行第 %d 次登录尝试...\n", i)

		base64Img, preLoginCookies, err := schoollogin.GetValidateCodeBase64()
		if err != nil {
			return "", fmt.Errorf("获取验证码失败: %v", err)
		}

		valCode, err := utils.RecognizeCaptcha(base64Img)
		if err != nil {
			return "", fmt.Errorf("验证码识别失败: %v", err)
		}
		log.Println("🤖 AI识别验证码为：", valCode)

		loginResult, err := schoollogin.Login(stuID, plainPassword, valCode, preLoginCookies)
		if err != nil {
			if strings.Contains(err.Error(), "验证码") {
				lastErr = err
				continue
			}
			return "", fmt.Errorf("登录失败: %w", err)
		}
		if loginResult == nil {
			return "", fmt.Errorf("登录失败: 无返回结果")
		}
		if loginResult.Response == nil {
			return "", fmt.Errorf("登录失败: 微学工接口未返回登录结果")
		}

		// ✅ 处理微学工平台提示（初始密码需修改）
		if !loginResult.Response.IsOk {
			if loginResult.Response.G != "" {
				return "", &LoginFailWithG{
					Err: fmt.Errorf("登录失败: 微学工平台提示信息: %s", loginResult.Response.Message),
					G:   loginResult.Response.G,
				}
			}

			// 如果提示验证码错误，尝试重试
			if strings.Contains(loginResult.Response.Message, "验证码") || strings.Contains(loginResult.Response.Message, "ValCode") {
				lastErr = fmt.Errorf("登录失败: %s", loginResult.Response.Message)
				continue
			}

			// 其它失败直接返回
			return "", fmt.Errorf("登录失败: %s", loginResult.Response.Message)
		}

		studentName, err := schoollogin.GetStudentNameFromDetail(loginResult.Cookies)
		if err != nil {
			log.Println("⚠️ 获取学生姓名失败，将使用空值。", err)
			studentName = ""
		} else {
			log.Printf("🎓 获取到学生姓名：%s\n", studentName)
		}

		err = database.SaveStudentOrUpdate(&database.Student{
			StuID:      stuID,
			Password:   plainPassword,
			Cookies:    utils.SerializeCookies(loginResult.Cookies),
			LastLogin:  time.Now(),
			Name:       studentName,
			AuthStatus: "valid",
		})
		if err != nil {
			return "", fmt.Errorf("保存学生信息失败: %v", err)
		}
		if err := database.DB.Model(&database.UserStudent{}).Where("stu_id = ?", stuID).Update("name", studentName).Error; err != nil {
			return "", fmt.Errorf("更新绑定姓名失败: %v", err)
		}
		return studentName, nil
	}
	return "", fmt.Errorf("多次尝试登录均失败: %v", lastErr)
}

func bindVerifiedStudent(userID int, stuID, name string) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var studentRecord database.Student
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&studentRecord, "stu_id = ?", stuID).Error; err != nil {
			return err
		}
		if ban, err := database.GetActiveStudentBan(tx, stuID, time.Now()); err == nil {
			return fmt.Errorf("%s", database.StudentBanMessage(*ban))
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var account database.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, userID).Error; err != nil {
			return err
		}
		if database.IsUserBanActive(account, time.Now()) {
			return errors.New(database.UserBanMessage(account))
		}
		var existing int64
		if err := tx.Model(&database.UserStudent{}).Where("user_id = ? AND stu_id = ?", userID, stuID).Count(&existing).Error; err != nil {
			return err
		}
		if existing == 0 {
			var count int64
			if err := tx.Model(&database.UserStudent{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
				return err
			}
			limit, label := bindingLimit(account.Role)
			if label == "" {
				return fmt.Errorf("未知用户角色")
			}
			if limit > 0 && count >= int64(limit) {
				return fmt.Errorf("%s最多只能绑定 %d 名学生", label, limit)
			}
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "stu_id"}}, DoNothing: true}).
			Create(&database.UserStudent{UserID: userID, StuID: stuID, Name: name}).Error
	})
}

func bindingLimit(role int) (int, string) {
	switch role {
	case database.RoleUser:
		return config.GetInt("student_limit_user", 2), "普通用户"
	case database.RoleSponsor:
		return config.GetInt("student_limit_sponsor", 12), "赞助用户"
	case database.RoleAdmin:
		return config.GetInt("student_limit_admin", 0), "管理员"
	case database.RoleSuperAdmin:
		return config.GetInt("student_limit_super_admin", 0), "超级管理员"
	default:
		return 0, ""
	}
}

// LoginWithoutBind 仅用于登录获取 cookies，不进行绑定
func LoginWithoutBind(stuID, plainPassword string) ([]*http.Cookie, error) {
	var lastErr error

	for i := 1; i <= config.GetInt("captcha_ai_attempts", 3); i++ {
		log.Printf("🔁 第 %d 次尝试登录学号 %s...\n", i, stuID)

		// 每次重试都必须重新获取验证码和预登录 cookies
		base64Img, preCookies, err := schoollogin.GetValidateCodeBase64()
		if err != nil {
			return nil, fmt.Errorf("获取验证码失败: %v", err)
		}

		valCode, err := utils.RecognizeCaptcha(base64Img)
		if err != nil {
			lastErr = fmt.Errorf("验证码识别代码层失败: %v", err)
			continue
		}

		loginResult, err := schoollogin.Login(stuID, plainPassword, valCode, preCookies)
		if err != nil {
			// 如果平台提示验证码问题，则再试
			if strings.Contains(err.Error(), "验证码") {
				lastErr = err
				log.Printf("⚠️ 第 %d 次登录验证码错误，重新识别中...\n", i)
				continue
			}
			return nil, fmt.Errorf("登录失败: %v", err)
		}

		// 核心修复：检测平台提示信息是否为验证码错误
		if loginResult == nil || loginResult.Response == nil || !loginResult.Response.IsOk {
			if loginResult == nil || loginResult.Response == nil {
				lastErr = fmt.Errorf("微学工接口未返回登录结果")
				continue
			}
			if strings.Contains(loginResult.Response.Message, "验证码") {
				lastErr = fmt.Errorf("登录失败: %s", loginResult.Response.Message)
				log.Printf("⚠️ 第 %d 次登录验证码错误，重新识别中...\n", i)
				continue
			}
			return nil, fmt.Errorf("登录失败: %s", loginResult.Response.Message)
		}

		if len(loginResult.Cookies) == 0 {
			lastErr = fmt.Errorf("登录失败: cookies 未获取到")
			continue
		}

		return loginResult.Cookies, nil
	}

	return nil, fmt.Errorf("多次登录失败: %v", lastErr)
}
