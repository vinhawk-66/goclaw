package voicebox

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// voiceboxCreds maps encrypted credentials from channel_instances.credentials.
type voiceboxCreds struct {
	SecretKey string `json:"secret_key,omitempty"`
}

// voiceboxInstanceConfig maps non-secret settings from channel_instances.config.
type voiceboxInstanceConfig struct {
	DMPolicy          string   `json:"dm_policy,omitempty"`
	AllowFrom         []string `json:"allow_from,omitempty"`
	AuthMode          string   `json:"auth_mode,omitempty"`
	TokenExpiry       int64    `json:"token_expiry,omitempty"`
	AllowedDevices    []string `json:"allowed_devices,omitempty"`
	STTProxyURL       string   `json:"stt_proxy_url,omitempty"`
	STTAPIKey         string   `json:"stt_api_key,omitempty"`
	STTTenantID       string   `json:"stt_tenant_id,omitempty"`
	STTTimeoutSeconds int      `json:"stt_timeout_seconds,omitempty"`
}

// Factory creates a voicebox channel from DB channel instance data.
func Factory(name string, creds json.RawMessage, cfg json.RawMessage,
	msgBus *bus.MessageBus, pairingSvc store.PairingStore) (channels.Channel, error) {

	var c voiceboxCreds
	if len(creds) > 0 {
		if err := json.Unmarshal(creds, &c); err != nil {
			return nil, fmt.Errorf("decode voicebox credentials: %w", err)
		}
	}

	var ic voiceboxInstanceConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &ic); err != nil {
			return nil, fmt.Errorf("decode voicebox config: %w", err)
		}
	}

	var auth *TokenAuth
	if ic.AuthMode == "token" {
		if strings.TrimSpace(c.SecretKey) == "" {
			return nil, fmt.Errorf("voicebox secret_key is required when auth_mode=token")
		}
		auth = NewTokenAuth(c.SecretKey, ic.TokenExpiry, ic.AllowedDevices)
	}

	ch := New(ChannelConfig{
		DMPolicy:  ic.DMPolicy,
		AllowFrom: ic.AllowFrom,
	}, msgBus, pairingSvc, auth, NewSTTProxy(ic.STTProxyURL, ic.STTAPIKey, ic.STTTenantID, ic.STTTimeoutSeconds))

	ch.SetName(name)
	return ch, nil
}
