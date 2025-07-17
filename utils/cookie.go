package utils

import (
	"net/http"
	"strings"
)

// SerializeCookies 将 []*http.Cookie 序列化成 HTTP 请求头 Cookie 字符串，形如：name1=value1; name2=value2（写）
func SerializeCookies(cookies []*http.Cookie) string {
	var parts []string
	for _, c := range cookies {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

// DeserializeCookies 反序列化字符串为 []*http.Cookie （读）
func DeserializeCookies(cookieStr string) ([]*http.Cookie, error) {
	var cookies []*http.Cookie
	pairs := strings.Split(cookieStr, "; ")
	for _, p := range pairs {
		if parts := strings.SplitN(p, "=", 2); len(parts) == 2 {
			cookies = append(cookies, &http.Cookie{
				Name:  parts[0],
				Value: parts[1],
			})
		}
	}
	return cookies, nil
}
