package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

type User struct {
	ID        string `json:"id"`
	GitHubID  int64  `json:"github_id"`
	Login     string `json:"login"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

type userClaims struct {
	GitHubID  int64  `json:"github_id"`
	Login     string `json:"login"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	jwt.RegisteredClaims
}

func NewJWTManager(secret []byte, ttl time.Duration) *JWTManager {
	return &JWTManager{secret: secret, ttl: ttl}
}

func (m *JWTManager) Sign(user User, now time.Time) (string, error) {
	if user.ID == "" {
		user.ID = fmt.Sprintf("github:%d", user.GitHubID)
	}
	claims := userClaims{
		GitHubID:  user.GitHubID,
		Login:     user.Login,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *JWTManager) Parse(tokenString string, now time.Time) (User, error) {
	claims := &userClaims{}
	oldTimeFunc := jwt.TimeFunc
	jwt.TimeFunc = func() time.Time { return now }
	defer func() { jwt.TimeFunc = oldTimeFunc }()

	token, err := jwt.ParseWithClaims(tokenString, claims, func(tk *jwt.Token) (interface{}, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return User{}, err
	}
	if !token.Valid {
		return User{}, fmt.Errorf("token invalid")
	}
	return User{
		ID:        claims.Subject,
		GitHubID:  claims.GitHubID,
		Login:     claims.Login,
		Name:      claims.Name,
		AvatarURL: claims.AvatarURL,
	}, nil
}
