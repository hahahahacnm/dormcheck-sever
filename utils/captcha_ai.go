package utils

import (
	"bytes"
	"dormcheck/config"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// RecognizeCaptcha 使用通义千问 API 识别 base64 格式验证码图像，返回识别结果字符串
func RecognizeCaptcha(base64Image string) (string, error) {
	apiKey := config.DashScopeAPIKey
	if apiKey == "" {
		return "", fmt.Errorf("DashScope API Key 未设置")
	}

	reqBody := map[string]interface{}{
		"model": "qwen-vl-ocr-latest", // 通义大模型·模型名称
		"messages": []map[string]interface{}{
			{
				"role": "system",
				"content": []map[string]string{
					{"type": "text", "text": "你被使用api调用，作用是验证码识别."},
				},
			},
			{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "image_url", "image_url": map[string]string{"url": base64Image}},
					{"type": "text", "text": "4位长度字符类型验证码图像识别，只输出识别结果"},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
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
