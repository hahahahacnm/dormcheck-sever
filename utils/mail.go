package utils

import (
	"bytes"
	"fmt"
	"html/template"
	"path/filepath"
	"time"

	"gopkg.in/gomail.v2"
)

// ========== 配置你的邮箱信息 ==========

const (
	SMTPHost     = "smtp.exmail.qq.com"
	SMTPPort     = 465
	SMTPUser     = "kiki@kikirepository.cn"
	SMTPPassword = "etujAmAzvM36cdek"
	FromName     = "DormCheck 系统"
)

// ========== 邮件模板数据结构 ==========

type MailTemplateData struct {
	Subject    string
	Body       template.HTML // ✅ template.HTML 防止转义
	ActionURL  string
	ActionText string
}

// ========== 渲染 HTML 模板 ==========

func renderTemplate(subject, body, actionURL, actionText string) (string, error) {
	tmplPath := filepath.Join("templates", "mail_template.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return "", err
	}

	// 如果 body 是原始 HTML，就不用解码了，直接用它即可

	data := MailTemplateData{
		Subject:    subject,
		Body:       template.HTML(body), // 只要body是原始HTML，这里就没问题
		ActionURL:  actionURL,
		ActionText: actionText,
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// ========== 发送邮件通用方法 ==========

func SendMail(to, subject, htmlBody, actionURL, actionText string) error {
	htmlContent, err := renderTemplate(subject, htmlBody, actionURL, actionText)
	if err != nil {
		return err
	}

	m := gomail.NewMessage()
	m.SetHeader("From", m.FormatAddress(SMTPUser, FromName))
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlContent)

	d := gomail.NewDialer(SMTPHost, SMTPPort, SMTPUser, SMTPPassword)
	d.SSL = true // QQ 邮箱必须使用 SSL

	return d.DialAndSend(m)
}

// ========== 发送验证码邮件（示例封装） ==========

func SendVerificationCodeEmail(to string, code string) error {
	html := fmt.Sprintf(`<p>您好，您的验证码是：<strong>%s</strong>，有效期为 15 分钟。</p>`, code)
	return SendMail(to, "邮箱验证", html, "", "")
}

// SendSignResultEmail 发送签到结果邮件通知
func SendSignResultEmail(to string, stuName, activityName string, success bool, errorMsg string, sendTime time.Time) error {
	var resultMsg string
	if success {
		resultMsg = `<p style="color: green;"><strong>✔️ 签到成功</strong></p>`
	} else {
		resultMsg = fmt.Sprintf(`<p style="color: red;"><strong>❌ 签到失败</strong></p><p>失败原因：%s</p>`, errorMsg)
	}

	timeStr := sendTime.Format("2006-01-02 15:04:05")

	html := fmt.Sprintf(`
		<p>您好，以下是 <strong>%s</strong> 的签到任务结果：</p>
		<p>活动名称：<strong>%s</strong></p>
		%s
		<p>发送时间：%s</p>
		<p>感谢您使用 DormCheck 签到平台。</p>
	`, stuName, activityName, resultMsg, timeStr)

	subject := "签到结果通知"

	return SendMail(to, subject, html, "", "")
}

// SendAccountErrorEmail 发送账号异常通知邮件
func SendAccountErrorEmail(to, stuName, stuId, errorMsg string, sendTime time.Time) error {
	timeStr := sendTime.Format("2006-01-02 15:04:05")

	html := fmt.Sprintf(`
		<p>您好，系统检测到您绑定的账号信息存在误差：</p>
		<p>学生姓名：<strong>%s</strong></p>
		<p>学号：<strong>%s</strong></p>
		<p style="color: red;"><strong>❌ 登录状态刷新失败</strong></p>
		<p>失败原因：%s</p>
		<p>请检查您的微学工账号密码是否已在 DormCheck 平台正确录入，并及时修改。</p>
		<p>如您无法解决问题，请加QQ群咨询：<strong>947767423</strong>。</p>
		<p>检测时间：%s</p>
		<p>感谢您使用 DormCheck 签到平台。</p>
	`, stuName, stuId, errorMsg, timeStr)

	subject := "微学工绑定异常提醒"

	return SendMail(to, subject, html, "", "")
}


