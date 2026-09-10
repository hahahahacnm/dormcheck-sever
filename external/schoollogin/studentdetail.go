package schoollogin

import (
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"html" // ✅ 用于 HTML 实体解码
)

// GetStudentNameFromDetail 使用已登录 cookies 从新首页提取学生姓名
func GetStudentNameFromDetail(cookies []*http.Cookie) (string, error) {
	url := "http://me.swmu.edu.cn/"

	// 构建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	// 请求头
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")

	// 拼接 Cookie
	var cookieStr string
	for _, ck := range cookies {
		cookieStr += ck.Name + "=" + ck.Value + "; "
	}
	req.Header.Set("Cookie", strings.TrimSpace(cookieStr))

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	htmlText := string(body)

	// ✅ 精准匹配第一个 usename 里的第一个 span（姓名）
	re := regexp.MustCompile(
		`<div\s+class="usename">\s*<span>(.*?)</span>\s*<span>【.*?】</span>`,
	)

	matches := re.FindStringSubmatch(htmlText)
	if len(matches) < 2 {
		log.Println("⚠️ 未能从新首页中提取学生姓名，Cookie 可能失效")
		return "", errors.New("无法从新页面中解析出 student name")
	}

	// ✅ HTML 实体解码（&#x9648; → 陈）
	rawName := strings.TrimSpace(matches[1])
	name := html.UnescapeString(rawName)

	log.Printf("✅ 成功提取 student name：%s\n", name)
	return name, nil
}
