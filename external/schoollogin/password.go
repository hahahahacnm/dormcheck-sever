package schoollogin

import (
	"dormcheck/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

const changePasswordURL = PlatBaseURL + "/Authentication/CHWPost"

type changePasswordResponse struct {
	IsOK    bool   `json:"isok"`
	Message string `json:"msg"`
}

// ChangeInitialPassword submits the password update required by the platform's
// weak-password flow. The platform expects RSA PKCS#1 v1.5 Base64 values.
func ChangeInitialPassword(g, oldPassword, newPassword string) error {
	if strings.TrimSpace(g) == "" || oldPassword == "" || newPassword == "" {
		return fmt.Errorf("修改密码参数不完整")
	}
	if strings.ContainsAny(g, "\r\n") {
		return fmt.Errorf("修改密码凭证无效")
	}

	encryptedOld, err := utils.EncryptWithRSA(oldPassword)
	if err != nil {
		return fmt.Errorf("加密原密码失败: %w", err)
	}
	encryptedNew, err := utils.EncryptWithRSA(newPassword)
	if err != nil {
		return fmt.Errorf("加密新密码失败: %w", err)
	}

	pageURL := PlatBaseURL + "/Authentication/CHW?g=" + url.QueryEscape(g)
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("初始化微学工会话失败")
	}
	client := newSchoolClient()
	client.Jar = jar
	pageReq, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return fmt.Errorf("创建密码修改页面请求失败")
	}
	pageReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	pageResp, err := client.Do(pageReq)
	if err != nil {
		return fmt.Errorf("获取微学工密码修改页面失败: %w", err)
	}
	if _, err := readPlatformResponse(pageResp); err != nil {
		return fmt.Errorf("获取微学工密码修改页面失败: %w", err)
	}
	if pageResp.StatusCode < 200 || pageResp.StatusCode >= 300 {
		return fmt.Errorf("微学工密码修改页面返回 HTTP %d", pageResp.StatusCode)
	}

	form := url.Values{
		"newPwd":   {encryptedNew},
		"passWord": {encryptedOld},
		"g":        {g},
	}
	req, err := http.NewRequest(http.MethodPost, changePasswordURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("创建密码修改请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Origin", PlatBaseURL)
	req.Header.Set("Referer", pageURL)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连接微学工密码修改接口失败: %w", err)
	}
	body, err := readPlatformResponse(resp)
	if err != nil {
		return fmt.Errorf("微学工密码修改请求失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("微学工密码修改接口返回 HTTP %d", resp.StatusCode)
	}
	var result changePasswordResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("无法解析微学工密码修改结果")
	}
	if !result.IsOK {
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = "平台未接受密码修改请求"
		}
		return fmt.Errorf("%s", message)
	}
	return nil
}
