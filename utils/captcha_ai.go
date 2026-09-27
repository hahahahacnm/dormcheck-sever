package utils

import (
	"bytes"
	"dormcheck/config"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RecognizeCaptcha 使用通义千问 API 识别 base64 格式验证码图像，返回识别结果字符串
func RecognizeCaptcha(base64Image string) (string, error) {
	apiKey := config.Get("captcha_ai_api_key")
	if apiKey == "" {
		return "", fmt.Errorf("验证码识别 AI API Key 尚未在后台配置")
	}
	endpoint := config.Get("captcha_ai_base_url")
	if endpoint == "" {
		return "", fmt.Errorf("验证码识别 AI 接口地址尚未配置")
	}

	reqBody := map[string]interface{}{
		"model": config.Get("captcha_ai_model"),
		"messages": []map[string]interface{}{
			{
				"role": "system",
				"content": []map[string]string{
					{"type": "text", "text": config.Get("captcha_ai_system_prompt")},
				},
			},
			{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "image_url", "image_url": map[string]string{"url": base64Image}},
					{"type": "text", "text": config.Get("captcha_ai_user_prompt")},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: time.Duration(config.GetInt("captcha_ai_timeout_seconds", 30)) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("验证码 AI 接口返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("未能识别出验证码")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}
