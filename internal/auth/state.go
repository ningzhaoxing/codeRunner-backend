package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type StateSigner struct {
	secret []byte
	ttl    time.Duration
}

type statePayload struct {
	Nonce    string `json:"nonce"`
	ReturnTo string `json:"return_to"`
	Exp      int64  `json:"exp"`
}

func NewStateSigner(secret []byte, ttl time.Duration) *StateSigner {
	return &StateSigner{secret: secret, ttl: ttl}
}

func (s *StateSigner) Sign(returnTo string, now time.Time) (string, error) {
	normalized, err := NormalizeReturnTo(returnTo)
	if err != nil {
		return "", err
	}
	nonce, err := randomBase64URL(18)
	if err != nil {
		return "", err
	}
	payload := statePayload{
		Nonce:    nonce,
		ReturnTo: normalized,
		Exp:      now.Add(s.ttl).Unix(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(data)
	sig := s.sign(payloadPart)
	return payloadPart + "." + sig, nil
}

func (s *StateSigner) Verify(state string, now time.Time) (string, error) {
	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid state format")
	}
	expectedSig := s.sign(parts[0])
	if !hmac.Equal([]byte(expectedSig), []byte(parts[1])) {
		return "", fmt.Errorf("invalid state signature")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode state payload: %w", err)
	}
	var payload statePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("parse state payload: %w", err)
	}
	if now.Unix() > payload.Exp {
		return "", fmt.Errorf("state expired")
	}
	return NormalizeReturnTo(payload.ReturnTo)
}

func (s *StateSigner) sign(payloadPart string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payloadPart))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
