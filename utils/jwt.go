package utils

import (
	"dormcheck/config"
	"dormcheck/database"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID       int
	TokenVersion int
	jwt.RegisteredClaims
}

// 生成 JWT，内含用户ID与当前token版本
func GenerateToken(user database.User) (string, error) {
	claims := Claims{
		UserID:       user.ID,
		TokenVersion: user.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)), // 30天有效期（为了保障用户体验）
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(config.JwtSecret)
}

// 验证 token 并校验 tokenVersion 是否与数据库一致
func ParseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return config.JwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("token无效")
	}

	// 查询数据库最新的 token_version 进行比对
	var user database.User
	err = database.DB.First(&user, claims.UserID).Error
	if err != nil {
		return nil, errors.New("用户不存在或数据库异常")
	}
	if user.TokenVersion != claims.TokenVersion {
		return nil, errors.New("token已失效，请重新登录")
	}

	return claims, nil
}
