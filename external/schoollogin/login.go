package schoollogin

import (
	"dormcheck/utils"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// LoginResponse 代表登录接口响应的数据结构
type LoginResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"msg"`
	IsOk    bool            `json:"isok"`
	Data    json.RawMessage `json:"data"`
	G       string          `json:"g"`
}

// LoginResult 封装登录成功后的关键信息
type LoginResult struct {
	Response *LoginResponse
	Cookies  []*http.Cookie
}

// Login 进行登录，返回封装好的登录结果和错误
func Login(username, password, valCode string, preCookies []*http.Cookie) (*LoginResult, error) {
	encUser, err := utils.EncryptWithRSA(username)
	if err != nil {
		log.Println("加密用户名失败:", err)
		return nil, err
	}
	encPass, err := utils.EncryptWithRSA(password)
	if err != nil {
		log.Println("加密密码失败:", err)
		return nil, err
	}

	form := url.Values{
		"LoginType":     {"0"},
		"UserName":      {encUser},
		"Password":      {encPass},
		"Remember":      {"true"},
		"ValCode":       {strings.ToLower(valCode)},
		"IsShowValCode": {"true"},
	}

	req, err := http.NewRequest("POST", "http://plat.swmu.edu.cn/MyAuthentication/put/", strings.NewReader(form.Encode()))
	if err != nil {
		log.Println("创建请求失败:", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var cookieStrings []string
	for _, ck := range preCookies {
		if ck.Name == "Vlis" || ck.Name == "VK_" {
			cookieStrings = append(cookieStrings, ck.Name+"="+ck.Value)
		}
	}
	if len(cookieStrings) > 0 {
		req.Header.Set("Cookie", strings.Join(cookieStrings, "; "))
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("请求发送失败:", err)
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("读取响应失败:", err)
		return nil, err
	}
	log.Println("获取登录响应:", string(bodyBytes))

	var loginResp LoginResponse
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		log.Println("解析响应体失败:", err)
		return nil, err
	}

	// 登录失败但 g 有效
	if !loginResp.IsOk && strings.TrimSpace(loginResp.G) != "" {
		gBytes, _ := json.Marshal(map[string]string{"g": loginResp.G})
		loginResp.Data = gBytes
		log.Println("微学工平台提示信息:", loginResp.Message)
		return &LoginResult{Response: &loginResp}, nil // ✅ 返回 nil 错误
	}

	// 登录失败且无 g
	if !loginResp.IsOk {
		log.Println("微学工平台提示信息:", loginResp.Message)
		return &LoginResult{Response: &loginResp}, nil
	}

	// 提取 ct_vali cookie
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

	if ctVali == "" {
		log.Println("未能获取有效的 ct_vali cookie")
		return nil, nil
	}

	finalCookies := []*http.Cookie{
		{Name: "qyuserid", Value: username},
		{Name: "utpstr", Value: "1"},
		{Name: "ct_vali", Value: ctVali},
	}

	return &LoginResult{
		Response: &loginResp,
		Cookies:  finalCookies,
	}, nil
}
