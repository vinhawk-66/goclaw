package voicebox

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func (c *Channel) serveWS(w http.ResponseWriter, r *http.Request) {
	if !websocket.IsWebSocketUpgrade(r) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Server is running\n"))
		return
	}

	deviceID := extractHeaderOrQuery(r, "Device-Id", "device-id")
	clientID := extractHeaderOrQuery(r, "Client-Id", "client-id")
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	protocolHeader := extractHeaderOrQuery(r, "Protocol-Version", "protocol-version")

	if strings.TrimSpace(deviceID) == "" {
		http.Error(w, "Device-Id required", http.StatusBadRequest)
		return
	}
	if !c.authorize(w, deviceID, clientID, authHeader) {
		return
	}

	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("voicebox websocket upgrade failed", "error", err)
		return
	}

	h := NewHandler(
		c,
		conn,
		deviceID,
		clientID,
		parseProtocolVersion(protocolHeader),
		r.URL.Query().Get("from") == "mqtt_gateway",
	)
	c.registerHandler(deviceID, h)

	runCtx := c.ctx
	if runCtx == nil {
		runCtx = context.Background()
	}
	go h.Run(runCtx)
}

func (c *Channel) authorize(w http.ResponseWriter, deviceID, clientID, authHeader string) bool {
	dmPolicy := c.config.DMPolicy
	if dmPolicy == "" {
		dmPolicy = defaultDMPolicy
	}

	switch dmPolicy {
	case "disabled":
		http.Error(w, "DM policy disabled", http.StatusForbidden)
		return false
	case "allowlist":
		if !c.IsAllowed(deviceID) {
			http.Error(w, "device not allowed", http.StatusForbidden)
			return false
		}
	case "pairing":
		if !c.ensurePairing(deviceID) {
			http.Error(w, "pairing required", http.StatusForbidden)
			return false
		}
	default:
		// open
	}

	if c.tokenAuth != nil {
		if err := c.tokenAuth.Verify(authHeader, clientID, deviceID); err != nil {
			slog.Warn("security.voicebox.auth_failed", "device_id", deviceID, "error", err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return false
		}
	}
	return true
}

func (c *Channel) ensurePairing(deviceID string) bool {
	if c.pairingService == nil {
		return c.IsAllowed(deviceID)
	}
	paired, err := c.pairingService.IsPaired(deviceID, c.Name())
	if err != nil {
		slog.Warn("security.pairing_check_failed, assuming paired (fail-open)",
			"device_id", deviceID, "channel", c.Name(), "error", err)
		return true
	}
	if paired {
		return true
	}
	if last, ok := c.pairingDebounce.Load(deviceID); ok {
		if time.Since(last.(time.Time)) < pairingDebounceTime {
			return false
		}
	}

	code, err := c.pairingService.RequestPairing(deviceID, c.Name(), deviceID, "default", nil)
	if err != nil {
		slog.Warn("voicebox pairing request failed", "device_id", deviceID, "error", err)
		return false
	}
	c.pairingDebounce.Store(deviceID, time.Now())
	slog.Info("voicebox pairing requested", "device_id", deviceID, "pairing_code", code)
	return false
}

func extractHeaderOrQuery(r *http.Request, header, query string) string {
	v := strings.TrimSpace(r.Header.Get(header))
	if v == "" {
		v = strings.TrimSpace(r.URL.Query().Get(query))
	}
	return v
}

func parseProtocolVersion(v string) int {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		switch n {
		case 1, 2, 3:
			return n
		}
	}
	return 1
}
