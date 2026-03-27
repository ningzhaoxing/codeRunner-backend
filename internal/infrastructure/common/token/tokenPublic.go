package token

import (
	"codeRunner-siwu/api/proto"
	"fmt"
	"os"

	"github.com/golang-jwt/jwt/v4"
	"github.com/sirupsen/logrus"
	"time"
)

type TokenIssuer interface {
	Public(request *proto.GenerateTokenRequest) (response *proto.GenerateTokenResponse, err error)
}

type token struct {
	jwtSecret []byte
	password  string
}

func NewToken() *token {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		logrus.Fatal("环境变量 JWT_SECRET 未设置")
	}
	password := os.Getenv("AUTH_PASSWORD")
	if password == "" {
		logrus.Fatal("环境变量 AUTH_PASSWORD 未设置")
	}
	return &token{
		jwtSecret: []byte(secret),
		password:  password,
	}
}

func (t *token) Public(request *proto.GenerateTokenRequest) (response *proto.GenerateTokenResponse, err error) {
	if request.Password != t.password {
		logrus.Error("infrastructure-token Public的验证失败")
		return response, fmt.Errorf("验证失败")
	}
	// 生成 JWT
	tk := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": request.Name,
		"exp":      time.Now().Add(time.Hour * 24).Unix(),
	})
	tokenString, err := tk.SignedString(t.jwtSecret)
	if err != nil {
		logrus.Error("infrastructure-token Public的token.SignedString 失败 err=%v", err)
		return response, fmt.Errorf("生成token失败")
	}
	response = new(proto.GenerateTokenResponse)
	response.Token = tokenString
	return response, nil
}

func (t *token) Verify(tokenString string) (ok bool, err error) {
	tk, err := jwt.Parse(tokenString, func(tk *jwt.Token) (interface{}, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodHMAC); !ok {
			logrus.Error("infrastructure-token Verify的token.Method失败 err=%v", fmt.Errorf("unexpected signing method"))
			return nil, fmt.Errorf("unexpected signing method")
		}
		return t.jwtSecret, nil
	})
	if err != nil {
		logrus.Error("infrastructure-token Verify的 jwt.Parse(失败 err=%v", err)
		return false, fmt.Errorf("token验证失败: %v", err)
	}
	if _, ok := tk.Claims.(jwt.MapClaims); ok && tk.Valid {
		return true, nil
	} else {
		return false, fmt.Errorf("token无效")
	}
}
