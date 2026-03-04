package voicebox

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTokenExpirySecs = 30 * 24 * 3600
	maxClockSkewSecs       = 120
)

// TokenAuth verifies xiaozhi-compatible authorization tokens.
type TokenAuth struct {
	secretKey      string
	expirySecs     int64
	allowedDevices map[string]struct{}
}

func NewTokenAuth(secretKey string, expirySecs int64, allowedDevices []string) *TokenAuth {
	if expirySecs <= 0 {
		expirySecs = defaultTokenExpirySecs
	}
	allow := make(map[string]struct{}, len(allowedDevices))
	for _, id := range allowedDevices {
		id = strings.TrimSpace(id)
		if id != "" {
			allow[id] = struct{}{}
		}
	}
	return &TokenAuth{secretKey: secretKey, expirySecs: expirySecs, allowedDevices: allow}
}

func (a *TokenAuth) Verify(token, clientID, deviceID string) error {
	if a == nil {
		return nil
	}
	if _, ok := a.allowedDevices[deviceID]; ok {
		return nil
	}
	if strings.TrimSpace(a.secretKey) == "" {
		return nil
	}

	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	if token == "" {
		return fmt.Errorf("empty token")
	}

	dot := strings.LastIndex(token, ".")
	if dot < 0 || dot == len(token)-1 {
		return fmt.Errorf("invalid token format")
	}
	sig := token[:dot]
	tsStr := token[dot+1:]
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid token timestamp: %w", err)
	}

	now := time.Now().Unix()
	if ts > now+maxClockSkewSecs {
		return fmt.Errorf("token timestamp in future")
	}
	if now-ts > a.expirySecs {
		return fmt.Errorf("token expired")
	}

	content := fmt.Sprintf("%s|%s|%d", clientID, deviceID, ts)
	mac := hmac.New(sha256.New, []byte(a.secretKey))
	_, _ = mac.Write([]byte(content))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return fmt.Errorf("invalid token signature")
	}
	return nil
}

// GenerateToken produces a token for manual provisioning and tests.
func GenerateToken(clientID, deviceID, secretKey string) string {
	ts := time.Now().Unix()
	content := fmt.Sprintf("%s|%s|%d", clientID, deviceID, ts)
	mac := hmac.New(sha256.New, []byte(secretKey))
	_, _ = mac.Write([]byte(content))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%d", sig, ts)
}
