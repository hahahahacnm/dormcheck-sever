package utils

import (
	"crypto/rand"
)

// GenerateVerificationCode 生成6位数字验证码
func GenerateVerificationCode() (string, error) {
	const digits = "0123456789"
	b := make([]byte, 6)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	for i := 0; i < 6; i++ {
		b[i] = digits[int(b[i])%len(digits)]
	}
	return string(b), nil
}
