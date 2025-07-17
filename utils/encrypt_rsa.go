package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
)

const MicroPlatformRSAPublicKey = `
-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQCC0hrRIjb3noDWNtbDpANbjt5I
wu2NFeDwU16Ec87ToqeoIm2KI+cOs81JP9aTDk/jkAlU97mN8wZkEMDr5utAZtMV
ht7GLX33Wx9XjqxUsDfsGkqNL8dXJklWDu9Zh80Ui2Ug+340d5dZtKtd+nv09QZq
GjdnSp9PTfFDBY133QIDAQAB
-----END PUBLIC KEY-----
`

// EncryptWithRSA 使用微学工平台的 RSA 公钥加密字符串
func EncryptWithRSA(plainText string) (string, error) {
	// 解析 PEM 格式的公钥
	block, _ := pem.Decode([]byte(MicroPlatformRSAPublicKey))
	if block == nil {
		return "", errors.New("无法解析 RSA 公钥")
	}

	// 将 block.Bytes 转换为 rsa.PublicKey 接口
	pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", err
	}
	pubKey, ok := pubInterface.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("公钥类型断言失败")
	}

	// 使用 PKCS1 v1.5 方式加密明文
	encryptedBytes, err := rsa.EncryptPKCS1v15(rand.Reader, pubKey, []byte(plainText))
	if err != nil {
		return "", err
	}

	// 返回 base64 编码的密文字符串
	return base64.StdEncoding.EncodeToString(encryptedBytes), nil
}
