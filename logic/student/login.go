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

// LoginFailWithG 封装登录失败信息和 g 值
type LoginFailWithG struct {
	Err error   // 原始错误信息
	G   string  // 初始密码修改标识
}

func (e *LoginFailWithG) Error() string {
	return e.Err.Error()
}

// LoginAndBindStudent 尝试登录微学工平台，并保存学生信息 + 用户绑定 + 姓名
func LoginAndBindStudent(userID int, stuID, plainPassword string) error {
	var lastErr error
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

	switch user.Role {
	case 1:
		if currentCount >= 2 {
			return fmt.Errorf("普通用户最多只能绑定 2 名学生")
		}
	case 2:
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

		loginResult, _ := schoollogin.Login(stuID, plainPassword, valCode, preLoginCookies)
		if loginResult == nil {
			return fmt.Errorf("登录失败: 无返回结果")
		}

		// ✅ 处理微学工平台提示（初始密码需修改）
		if !loginResult.Response.IsOk {
			if loginResult.Response.G != "" {
				return &LoginFailWithG{
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
			return fmt.Errorf("登录失败: %s", loginResult.Response.Message)
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

		// 🚨 核心防护：cookies 为空也算失败
		if loginResult == nil || loginResult.Cookies == nil || len(loginResult.Cookies) == 0 {
			lastErr = fmt.Errorf("登录成功但未获取到 cookies")
			continue
		}

		return loginResult.Cookies, nil
	}

	return nil, fmt.Errorf("多次登录失败: %v", lastErr)
}
