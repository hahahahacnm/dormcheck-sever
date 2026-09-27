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
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(config.GetInt("jwt_expiration_hours", 720)) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(config.JWTSecret())
}

// 验证 token 并校验 tokenVersion 是否与数据库一致
func ParseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return config.JWTSecret(), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.UserID <= 0 {
		return nil, errors.New("token无效")
	}
	return claims, nil
}
