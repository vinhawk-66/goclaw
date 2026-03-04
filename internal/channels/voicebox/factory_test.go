package voicebox

import (
	"encoding/json"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
)

func TestFactory_TokenModeRequiresSecret(t *testing.T) {
	cfg := map[string]any{"auth_mode": "token"}
	cfgJSON, _ := json.Marshal(cfg)

	_, err := Factory("voicebox", nil, cfgJSON, bus.New(), nil)
	if err == nil {
		t.Fatalf("expected error when auth_mode=token and secret_key is missing")
	}
}
