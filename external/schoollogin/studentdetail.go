package schoollogin

import (
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
)

// GetStudentNameFromDetail 使用已登录的 cookies 请求学生详情页，从 HTML 中提取 userName
func GetStudentNameFromDetail(cookies []*http.Cookie) (string, error) {
	url := "http://me.swmu.edu.cn/studentwork/StudentManager/Detail"

	// 构建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	// 设置请求头
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")

	// 手动拼接 Cookie 头
	var cookieStr string
	for _, ck := range cookies {
		cookieStr += ck.Name + "=" + ck.Value + "; "
	}
	req.Header.Set("Cookie", strings.TrimSpace(cookieStr))

	// 发起请求
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

	html := string(body)

	// debug 打印返回内容（可注释掉）
	// log.Println("学生详情页面返回 HTML：\n", html)

	// 正则匹配 userName
	re := regexp.MustCompile(`var\s+userName\s*=\s*'([^']+)'`)
	matches := re.FindStringSubmatch(html)
	if len(matches) < 2 {
		log.Println("⚠️ 未能在页面中提取 userName。可能是 Cookie 失效。")
		return "", errors.New("无法从页面中解析出 userName")
	}

	name := strings.TrimSpace(matches[1])
	log.Printf("✅ 成功提取 student name：%s\n", name)

	return name, nil
}
