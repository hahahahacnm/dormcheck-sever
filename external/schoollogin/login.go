// logic/schoollogin/login.go
package schoollogin

import (
	"dormcheck/utils"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// LoginResponse 代表登录接口响应的数据结构（可根据实际API调整）
type LoginResponse struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
	IsOk    bool   `json:"isok"`
	Data    struct {
		//这里是登录接口可能返回的其他变量，可以在这里存储
	} `json:"data"`
}

// LoginResult 封装登录成功后的关键信息
type LoginResult struct {
	Response *LoginResponse
	Cookies  []*http.Cookie
}

// Login 进行登录，返回封装好的登录结果和错误
func Login(username, password, valCode string, preCookies []*http.Cookie) (*LoginResult, error) {
	// 1. RSA 加密用户名和密码
	encUser, err := utils.EncryptWithRSA(username)
	if err != nil {
		return nil, fmt.Errorf("加密用户名失败: %v", err)
	}
	encPass, err := utils.EncryptWithRSA(password)
	if err != nil {
		return nil, fmt.Errorf("加密密码失败: %v", err)
	}

	// 2. 使用 url.Values 构造登录请求体（表单格式）
	form := url.Values{
		"LoginType":     {"0"},
		"UserName":      {encUser},
		"Password":      {encPass},
		"Remember":      {"true"},
		"ValCode":       {strings.ToLower(valCode)},
		"IsShowValCode": {"true"},
	}

	// 3. 创建请求
	req, err := http.NewRequest("POST", "http://plat.swmu.edu.cn/MyAuthentication/put/", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 4. 添加请求头和 cookie
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var cookieStrings []string
	for _, ck := range preCookies {
		if ck.Name == "Vlis" || ck.Name == "VK_" {
			cookieStrings = append(cookieStrings, ck.Name+"="+ck.Value)
		}
	}
	if len(cookieStrings) > 0 {
		cookieHeader := strings.Join(cookieStrings, "; ")
		req.Header.Set("Cookie", cookieHeader)
	}

	// 5. 发起请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求发送失败: %v", err)
	}
	defer resp.Body.Close()

	// 6. 读取响应
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}
	// 🐞 打印响应内容
	log.Println("获取登录响应:", string(bodyBytes))

	// 7. 解析返回 JSON
	var loginResp LoginResponse
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		return nil, fmt.Errorf("解析响应体失败: %v", err)
	}

	// 8. 登录失败检查
	if !loginResp.IsOk {
		return &LoginResult{Response: &loginResp}, fmt.Errorf("微学工平台提示信息: %s", loginResp.Message)
	}

	// 9. 提取 Cookies
	var ctVali string
	ctValiCount := 0
	for _, setCookie := range resp.Header["Set-Cookie"] {
		if strings.HasPrefix(setCookie, "ct_vali=") {
			ctValiCount++
			if ctValiCount == 2 {
				parts := strings.SplitN(setCookie, ";", 2)
				ctVali = strings.TrimPrefix(parts[0], "ct_vali=")
				break
			}
		}
	}

	// 确保获取到有效的 Cookies
	if ctVali == "" {
		return nil, fmt.Errorf("未能获取有效的 ct_vali cookie")
	}

	// 10. 构造完整的登录态 Cookies
	finalCookies := []*http.Cookie{
		{Name: "qyuserid", Value: username},
		{Name: "utpstr", Value: "1"},
		{Name: "ct_vali", Value: ctVali},
	}

	// 返回封装的登录结果，包括正确的 Cookies
	return &LoginResult{
		Response: &loginResp,
		Cookies:  finalCookies,
	}, nil
}
