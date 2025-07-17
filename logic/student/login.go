// logic/student/login.go
package student

import (
	"dormcheck/database"
	"dormcheck/external/schoollogin"
	"dormcheck/utils"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// LoginAndBindStudent 尝试登录微学工平台，并保存学生信息 + 用户绑定 + 姓名
func LoginAndBindStudent(userID int, stuID, plainPassword string) error {
	var lastErr error
	db := database.DB

	// 🧠 新增：判断绑定数量限制
	var user database.User
	if err := db.First(&user, userID).Error; err != nil {
		return fmt.Errorf("用户不存在")
	}

	var currentCount int64
	if err := db.Model(&database.UserStudent{}).Where("user_id = ?", userID).Count(&currentCount).Error; err != nil {
		return fmt.Errorf("查询已绑定学生失败: %v", err)
	}

	switch user.Role {
	case 1: // 普通用户
		if currentCount >= 2 {
			return fmt.Errorf("普通用户最多只能绑定 2 名学生")
		}
	case 2: // 赞助用户
		if currentCount >= 12 {
			return fmt.Errorf("赞助用户最多只能绑定 12 名学生")
		}
	case 0:
		// 管理员无限制
	default:
		return fmt.Errorf("未知用户角色")
	}

	for i := 1; i <= 3; i++ {
		log.Printf("🔁 正在进行第 %d 次登录尝试...\n", i)

		base64Img, preLoginCookies, err := schoollogin.GetValidateCodeBase64()
		if err != nil {
			return fmt.Errorf("获取验证码失败: %v", err)
		}

		valCode, err := utils.RecognizeCaptcha(base64Img)
		if err != nil {
			return fmt.Errorf("验证码识别失败: %v", err)
		}
		log.Println("🤖 AI识别验证码为：", valCode)

		loginResult, err := schoollogin.Login(stuID, plainPassword, valCode, preLoginCookies)
		if err != nil {
			fmt.Println("⚠️ 登录失败:", err)

			if strings.Contains(err.Error(), "验证码") || strings.Contains(err.Error(), "ValCode") {
				lastErr = err
				continue
			}
			return fmt.Errorf("登录失败: %v", err)
		}

		studentName, err := schoollogin.GetStudentNameFromDetail(loginResult.Cookies)
		if err != nil {
			log.Println("⚠️ 获取学生姓名失败，将使用空值。", err)
			studentName = ""
		} else {
			log.Printf("🎓 获取到学生姓名：%s\n", studentName)
		}

		err = database.SaveStudentOrUpdate(&database.Student{
			StuID:     stuID,
			Password:  plainPassword,
			Cookies:   utils.SerializeCookies(loginResult.Cookies),
			LastLogin: time.Now(),
			Name:      studentName,
		})
		if err != nil {
			return fmt.Errorf("保存学生信息失败: %v", err)
		}

		err = database.BindUserAndStudent(userID, stuID, studentName)
		if err != nil {
			return fmt.Errorf("用户与学号绑定失败: %v", err)
		}

		return nil
	}

	return fmt.Errorf("多次尝试登录均失败: %v", lastErr)
}

// LoginWithoutBind 仅用于登录获取 cookies，不进行绑定
func LoginWithoutBind(stuID, plainPassword string) ([]*http.Cookie, error) {
	var lastErr error

	for i := 1; i <= 3; i++ {
		log.Printf("🔁 第 %d 次尝试登录学号 %s...\n", i, stuID)

		// 获取验证码图像
		base64Img, preCookies, err := schoollogin.GetValidateCodeBase64()
		if err != nil {
			return nil, fmt.Errorf("获取验证码失败: %v", err)
		}

		valCode, err := utils.RecognizeCaptcha(base64Img)
		if err != nil {
			return nil, fmt.Errorf("验证码识别失败: %v", err)
		}

		// 登录请求
		loginResult, err := schoollogin.Login(stuID, plainPassword, valCode, preCookies)
		if err != nil {
			if strings.Contains(err.Error(), "验证码") {
				lastErr = err
				continue
			}
			return nil, fmt.Errorf("登录失败: %v", err)
		}

		return loginResult.Cookies, nil
	}

	return nil, fmt.Errorf("多次登录失败: %v", lastErr)
}
