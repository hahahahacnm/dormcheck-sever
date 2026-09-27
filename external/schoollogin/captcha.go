package schoollogin

import (
	"dormcheck/config"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"

	"golang.org/x/net/publicsuffix"
)

// GetValidateCodeBase64 返回 base64 验证码图像（带 data URI 前缀）、包含 Vlis 和 VK_ 的 cookies
func GetValidateCodeBase64() (base64Img string, cookies []*http.Cookie, err error) {
	// 当前13位毫秒时间戳
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)
	url := fmt.Sprintf("%s?v=%d", CaptchaURL, timestamp)

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return "", nil, fmt.Errorf("创建cookiejar失败: %w", err)
	}

	client := &http.Client{
		Jar:           jar,
		Timeout:       time.Duration(config.GetInt("school_captcha_timeout_seconds", 10)) * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	// The captcha endpoint is HTTPS and establishes the cookies consumed by Login.
	resp, err := client.Get(url)
	if err != nil {
		return "", nil, fmt.Errorf("请求验证码失败: %w", err)
	}
	defer resp.Body.Close()

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("读取验证码图片失败: %w", err)
	}

	// 带前缀的 base64 字符串，方便直接用作 img src
	base64Img = "data:image/gif;base64," + base64.StdEncoding.EncodeToString(imgBytes)

	// 过滤 cookie，只返回 Vlis 和 VK_
	allCookies := jar.Cookies(resp.Request.URL)
	for _, c := range allCookies {
		if c.Name == "Vlis" || c.Name == "VK_" {
			cookies = append(cookies, c)
		}
	}
	if len(cookies) < 2 {
		return "", nil, errors.New("未能获取到完整的 Vlis 和 VK_ cookies")
	}

	return base64Img, cookies, nil
}
