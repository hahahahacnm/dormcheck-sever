package utils

import (
	"bytes"
	"dormcheck/config"
	"dormcheck/templates"
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strings"
	"time"

	"gopkg.in/gomail.v2"
)

// ========== 邮件模板数据结构 ==========

type MailTemplateData struct {
	Subject    string
	Body       template.HTML // Only HTML assembled by the mail helpers below.
	ActionURL  string
	ActionText string
}

// ========== 渲染 HTML 模板 ==========

func renderTemplate(subject, body, actionURL, actionText string) (string, error) {
	tmpl, err := template.ParseFS(templates.FS, "mail_template.html")
	if err != nil {
		return "", err
	}

	data := MailTemplateData{
		Subject:    subject,
		Body:       template.HTML(body),
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
	smtpHost := config.Get("smtp_host")
	smtpPort := config.GetInt("smtp_port", 465)
	smtpUser := config.Get("smtp_username")
	smtpPassword := config.Get("smtp_password")
	if smtpHost == "" || smtpPort <= 0 || strings.TrimSpace(smtpUser) == "" || smtpPassword == "" {
		return fmt.Errorf("邮件服务尚未在超级管理员后台完成配置")
	}
	fromName := config.Get("smtp_from_name")
	if fromName == "" {
		fromName = "DormCheck 系统"
	}

	htmlContent, err := renderTemplate(subject, htmlBody, actionURL, actionText)
	if err != nil {
		return err
	}

	m := gomail.NewMessage()
	m.SetHeader("From", m.FormatAddress(smtpUser, fromName))
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	// Inbox previews commonly prefer text/plain. A real plain-text part also
	// prevents clients from showing raw HTML entities such as &#34;.
	m.SetBody("text/plain", mailPlainText(htmlBody))
	m.AddAlternative("text/html", htmlContent)

	d := gomail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPassword)
	d.SSL = config.GetBool("smtp_ssl", true)

	return d.DialAndSend(m)
}

var mailTags = regexp.MustCompile(`<[^>]*>`)
var mailLineBreaks = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>`)

func mailPlainText(body string) string {
	body = mailLineBreaks.ReplaceAllString(body, "\n")
	body = mailTags.ReplaceAllString(body, "")
	return strings.TrimSpace(html.UnescapeString(body))
}

// ========== 发送验证码邮件（示例封装） ==========

func SendVerificationCodeEmail(to string, code string) error {
	ttl := config.GetInt("email_code_ttl_minutes", 15)
	body := fmt.Sprintf(`<p>您好，您的验证码是：<strong>%s</strong>，有效期为 %d 分钟。</p>`, html.EscapeString(code), ttl)
	return SendMail(to, "邮箱验证", body, "", "")
}

// SendSignResultEmail 发送签到结果邮件通知
func SendSignResultEmail(to string, stuName, activityName string, success bool, errorMsg string, sendTime time.Time) error {
	var resultMsg string
	if success {
		resultMsg = `<p style="color: green;"><strong>✔️ 签到成功</strong></p>`
	} else {
		resultMsg = fmt.Sprintf(`<p style="color: red;"><strong>❌ 签到失败</strong></p><p>失败原因：%s</p>`, html.EscapeString(errorMsg))
	}

	timeStr := sendTime.Format("2006-01-02 15:04:05")

	html := fmt.Sprintf(`
		<p>您好，以下是 <strong>%s</strong> 的签到任务结果：</p>
		<p>活动名称：<strong>%s</strong></p>
		%s
		<p>发送时间：%s</p>
		<p>感谢您使用 DormCheck 自动化托管平台。</p>
	`, html.EscapeString(stuName), html.EscapeString(activityName), resultMsg, timeStr)

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
		<p>请检查您的微学工账号密码是否已在 DormCheck 平台正确录入，并及时修改。连续登录失败期间每天最多提醒一次；满 7 天后停止邮件提醒并锁定该学生的自动任务。</p>
		<p>如您无法解决问题，请加QQ群咨询：<strong>947767423</strong>。</p>
		<p>检测时间：%s</p>
		<p>感谢您使用 DormCheck 自动化托管平台。</p>
	`, html.EscapeString(stuName), html.EscapeString(stuId), html.EscapeString(errorMsg), timeStr)

	subject := "微学工绑定异常提醒"

	return SendMail(to, subject, html, "", "")
}

func SendActivityIssueEmail(to, stuName, stuId, activityID, activityName, issue string, sendTime time.Time) error {
	timeStr := sendTime.Format("2006-01-02 15:04:05")
	htmlBody := fmt.Sprintf(`
		<p>您好，系统检测到您托管的签到活动状态异常：</p>
		<p>学生姓名：<strong>%s</strong></p>
		<p>学号：<strong>%s</strong></p>
		<p>活动：<strong>%s</strong>（ID：%s）</p>
		<p style="color: #b42318;"><strong>%s</strong></p>
		<p>请在任务管理中查看当前状态。若任务由系统自动暂停，且活动与学生账号恢复正常，系统会恢复此前运行中的任务；已删除的任务需要重新创建。</p>
		<p>检测时间：%s</p>
		<p>感谢您使用 DormCheck 自动化托管平台。</p>
	`, html.EscapeString(stuName), html.EscapeString(stuId), html.EscapeString(activityName), html.EscapeString(activityID), html.EscapeString(issue), timeStr)
	return SendMail(to, "签到活动状态异常提醒", htmlBody, "", "")
}

func SendActivityRecoveryEmail(to, stuName, stuID, activityID, activityName string, sendTime time.Time) error {
	body := fmt.Sprintf(`<p>您好，学生 <strong>%s</strong>（学号：%s）的签到活动 <strong>%s</strong>（ID：%s）已恢复正常。</p><p>此前由系统自动暂停的任务已恢复运行，请在任务管理中查看当前计划。</p><p>检测时间：%s</p>`, html.EscapeString(stuName), html.EscapeString(stuID), html.EscapeString(activityName), html.EscapeString(activityID), sendTime.Format("2006-01-02 15:04:05"))
	return SendMail(to, "签到活动恢复提醒", body, "", "")
}

func SendAccountTasksPausedEmail(to, stuName, stuID string, sendTime time.Time) error {
	body := fmt.Sprintf(`<p>您好，学生 <strong>%s</strong>（学号：%s）的微学工登录状态已连续失效 7 天，关联托管任务已锁定。</p><p>请在学生绑定页面更新密码并重新验证，验证通过后此前由系统锁定且活动正常的任务会自动恢复。</p><p>检测时间：%s</p>`, html.EscapeString(stuName), html.EscapeString(stuID), sendTime.Format("2006-01-02 15:04:05"))
	return SendMail(to, "托管任务锁定提醒", body, "", "")
}

func SendSystemAlertEmail(to, issue string, sendTime time.Time) error {
	body := fmt.Sprintf(`<p>您好，DormCheck 后台巡检发现需要管理员关注的问题：</p><p style="color: #b42318;"><strong>%s</strong></p><p>检测时间：%s</p>`, html.EscapeString(issue), sendTime.Format("2006-01-02 15:04:05"))
	return SendMail(to, "平台巡检异常提醒", body, "", "")
}
