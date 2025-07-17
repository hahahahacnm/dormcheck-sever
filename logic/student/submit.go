package student

import (
	"dormcheck/database"
	"dormcheck/utils"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ExecuteSignTask 执行一次签到任务，并更新状态和错误信息
func ExecuteSignTask(task *database.Task) error {
	var updateAndReturn = func(status string, errMsg string) error {
		task.ExecStatus = status
		task.LastError = errMsg
		task.RetryCount++
		task.ExecutedAt = time.Now()

		// 保存任务状态
		if err := database.DB.Save(task).Error; err != nil {
			log.Printf("保存任务状态失败: %v\n", err)
		}

		// 如果有通知邮箱且不为空，则异步发送邮件
		if task.NotifyEmail != "" {
			go func() {
				err := utils.SendSignResultEmail(
					task.NotifyEmail,
					task.Name,
					task.ActivityName,
					status == "success",
					errMsg,
					time.Now(), // ✅ 补充发送时间
				)
				if err != nil {
					log.Printf("发送签到结果邮件失败: %v\n", err)
				}
			}()
		}

		if errMsg != "" {
			return fmt.Errorf("%s", errMsg)
		}
		return nil
	}

	// 查询学生信息
	var stu database.Student
	if err := database.DB.First(&stu, "stu_id = ?", task.StuID).Error; err != nil {
		return updateAndReturn("failed", fmt.Sprintf("找不到学号 %s 对应的学生信息", task.StuID))
	}
	if stu.Cookies == "" {
		return updateAndReturn("failed", "用户未登录或 Cookie 缺失")
	}

	// 解析 cookie
	cookies, err := utils.DeserializeCookies(stu.Cookies)
	if err != nil {
		return updateAndReturn("failed", fmt.Sprintf("cookie 解析失败: %v", err))
	}

	// 构造请求
	form := url.Values{
		"ActivityId":     {task.ActivityID},
		"ReasonText":     {""},
		"guidValue":      {""},
		"address":        {task.Address},
		"longitudeGaoDe": {fmt.Sprintf("%.6f", task.Longitude)},
		"latitudeGaoDe":  {fmt.Sprintf("%.5f", task.Latitude)},
		"RType":          {"1"},
	}
	req, err := http.NewRequest("POST", "http://plat.swmu.edu.cn/studentwork/PunchMStudent/SubmitSignin", strings.NewReader(form.Encode()))
	if err != nil {
		return updateAndReturn("failed", fmt.Sprintf("请求构造失败: %v", err))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	// 添加 Cookie
	var cookieStrs []string
	for _, ck := range cookies {
		cookieStrs = append(cookieStrs, ck.Name+"="+ck.Value)
	}
	req.Header.Set("Cookie", strings.Join(cookieStrs, "; "))

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return updateAndReturn("failed", fmt.Sprintf("请求发送失败: %v", err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return updateAndReturn("failed", fmt.Sprintf("响应解析失败: %v", err))
	}

	// 提取结果
	isOK, _ := result["isok"].(bool)
	msg, _ := result["msg"].(string)

	// 判断状态
	if isOK || msg == "该活动已经签到成功" {
		return updateAndReturn("success", "")
	} else {
		return updateAndReturn("failed", msg)
	}
}
