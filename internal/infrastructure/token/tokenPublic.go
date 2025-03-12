package token

import (
	"codeRunner-siwu/api/proto"
	"fmt"
	"github.com/golang-jwt/jwt/v4"
	"time"
)

type TokenIssuer interface {
	Public(request *proto.GenerateTokenRequest) (response *proto.GenerateTokenResponse, err error)
}

type token struct {
	JwtSecret []byte
}

func NewToken(jwtSecret []byte) *token {
	return &token{JwtSecret: jwtSecret}
}

func (t *token) Public(request *proto.GenerateTokenRequest) (response *proto.GenerateTokenResponse, err error) {
	if request.Password != "123456" {
		return response, fmt.Errorf("验证失败")
	}
	// 生成 JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": request.Name,
		"exp":      time.Now().Add(time.Hour * 24).Unix(),
	})
	tokenString, err := token.SignedString(t.JwtSecret)
	if err != nil {
		return response, fmt.Errorf("生成token失败")
	}
	response.Token = tokenString
	return response, nil
}

func (t *token) Verify(tokenString string) (ok bool, err error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// 检查签名方法是否正确
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		// 返回用于验证的密钥
		return t.JwtSecret, nil
	})
	if err != nil {
		return false, fmt.Errorf("token验证失败: %v", err)
	}
	// 检查 token 是否有效
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		fmt.Println("Token is valid. Claims:", claims)
	} else {
		return false, fmt.Errorf("token无效")
	}
	return true, nil
}
