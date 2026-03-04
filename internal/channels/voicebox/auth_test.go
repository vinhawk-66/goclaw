package voicebox

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestTokenAuthVerify_ValidToken(t *testing.T) {
	clientID := "client-1"
	deviceID := "AA:BB"
	secret := "top-secret"

	token := GenerateToken(clientID, deviceID, secret)
	auth := NewTokenAuth(secret, 3600, nil)
	if err := auth.Verify("Bearer "+token, clientID, deviceID); err != nil {
		t.Fatalf("expected token to be valid: %v", err)
	}
}

func TestTokenAuthVerify_Expired(t *testing.T) {
	clientID := "client-1"
	deviceID := "AA:BB"
	secret := "top-secret"
	ts := time.Now().Add(-48 * time.Hour).Unix()
	token := signedToken(clientID, deviceID, secret, ts)

	auth := NewTokenAuth(secret, 60, nil)
	err := auth.Verify(token, clientID, deviceID)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired error, got: %v", err)
	}
}

func TestTokenAuthVerify_WhitelistBypass(t *testing.T) {
	auth := NewTokenAuth("secret", 3600, []string{"AA:BB"})
	if err := auth.Verify("", "client", "AA:BB"); err != nil {
		t.Fatalf("expected whitelist bypass: %v", err)
	}
}

func signedToken(clientID, deviceID, secret string, ts int64) string {
	content := fmt.Sprintf("%s|%s|%d", clientID, deviceID, ts)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(content))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%d", sig, ts)
}
