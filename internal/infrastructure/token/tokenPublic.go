package token

import (
	"codeRunner-siwu/api/proto"
	"fmt"
	"github.com/golang-jwt/jwt/v4"
	"log"
	"time"
)

type TokenIssuer interface {
	Public(request *proto.GenerateTokenRequest) (response *proto.GenerateTokenResponse, err error)
}

type token struct {
}

func NewToken() *token {
	return &token{}
}

func (t *token) Public(request *proto.GenerateTokenRequest) (response *proto.GenerateTokenResponse, err error) {
	JwtSecret := []byte("I'm codeRunner")
	if request.Password != "123456" {
		log.Printf("infrastructure-token Public的验证失败")
		return response, fmt.Errorf("验证失败")
	}
	// 生成 JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": request.Name,
		"exp":      time.Now().Add(time.Hour * 24).Unix(),
	})
	tokenString, err := token.SignedString(JwtSecret)
	if err != nil {
		log.Printf("infrastructure-token Public的token.SignedString 失败 err=%v", err)
		return response, fmt.Errorf("生成token失败")
	}
	response.Token = tokenString
	return response, nil
}

func (t *token) Verify(tokenString string) (ok bool, err error) {
	JwtSecret := []byte("I'm codeRunner")
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// 检查签名方法是否正确
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			log.Printf("infrastructure-token Verify的token.Method失败 err=%v", fmt.Errorf("unexpected signing method"))
			return nil, fmt.Errorf("unexpected signing method")
		}
		// 返回用于验证的密钥
		return JwtSecret, nil
	})
	if err != nil {
		log.Printf("infrastructure-token Verify的 jwt.Parse(失败 err=%v", err)
		return false, fmt.Errorf("token验证失败: %v", err)
	}
	// 检查 token 是否有效
	if _, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return true, nil
	} else {
		return false, fmt.Errorf("token无效")
	}
}
