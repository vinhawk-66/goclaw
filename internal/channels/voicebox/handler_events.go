package voicebox

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
)

func (h *Handler) handleTextMessage(ctx context.Context, data []byte) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Debug("voicebox: invalid json payload", "device", h.deviceID, "error", err)
		return
	}

	switch msg.Type {
	case "listen":
		var listen ListenMessage
		if err := json.Unmarshal(data, &listen); err != nil {
			slog.Debug("voicebox: invalid listen payload", "device", h.deviceID, "error", err)
			return
		}
		h.handleListen(ctx, listen)
	case "abort":
		h.handleAbort()
	case "mcp":
		h.handleMCP(data)
	default:
		slog.Debug("voicebox: unsupported message", "device", h.deviceID, "type", msg.Type)
	}
}

func (h *Handler) handleBinaryMessage(ctx context.Context, data []byte) {
	if h.mqttGateway {
		data = StripMQTTHeader(data)
	}
	frame, err := ParseBinaryFrame(data, h.protocolVersion)
	if err != nil {
		slog.Debug("voicebox: bad binary frame", "device", h.deviceID, "error", err)
		return
	}

	switch frame.Type {
	case FrameTypeJSON:
		h.handleTextMessage(ctx, frame.Payload)
	case FrameTypeAudio:
		h.mu.Lock()
		listening := h.listening
		h.mu.Unlock()
		if listening {
			h.audio.Append(frame.Payload)
		}
	}
}

func (h *Handler) handleListen(ctx context.Context, msg ListenMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.session == nil {
		return
	}

	switch msg.State {
	case "start", "detect":
		h.audio.Reset()
		h.listening = true
		h.session.Transition(StateListening)
		if msg.State == "detect" && msg.Text != "" {
			slog.Debug("voicebox: wake-word detected", "device", h.deviceID, "text", msg.Text)
		}
	case "stop":
		h.listening = false
		h.session.Transition(StateIdle)
		audio := h.audio.Bytes()
		h.audio.Reset()
		go h.transcribeAndPublish(ctx, audio)
	}
}

func (h *Handler) transcribeAndPublish(ctx context.Context, audio []byte) {
	if len(audio) == 0 {
		return
	}
	text, err := h.channel.sttProxy.Transcribe(ctx, audio)
	if err != nil {
		slog.Warn("voicebox: stt failed", "device", h.deviceID, "error", err)
		_ = h.writeJSON(AlertMessage{Type: "alert", Status: "error", Message: "speech recognition failed", Emotion: "triangle_exclamation"})
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if err := h.writeJSON(STTMessage{SessionID: h.sessionID, Type: "stt", Text: text}); err != nil {
		slog.Warn("voicebox: stt echo failed", "device", h.deviceID, "error", err)
	}

	h.channel.HandleMessage(
		h.deviceID,
		h.deviceID,
		text,
		nil,
		map[string]string{
			"platform":         "voicebox",
			"client_id":        h.clientID,
			"session_id":       h.sessionID,
			"protocol_version": strconvItoa(h.protocolVersion),
		},
		"direct",
	)
}

func (h *Handler) handleAbort() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ttsSender != nil {
		h.ttsSender.Cancel()
	}
	if h.session != nil {
		h.session.Transition(StateIdle)
	}
	_ = h.writeJSON(TTSMessage{SessionID: h.sessionID, Type: "tts", State: "stop"})
}

func (h *Handler) handleMCP(data []byte) {
	h.mu.Lock()
	session := h.session
	h.mu.Unlock()
	if session == nil || !session.MCPEnabled() {
		slog.Warn("voicebox: mcp payload ignored", "device", h.deviceID, "reason", "mcp not enabled")
		return
	}

	var msg MCPMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Debug("voicebox: invalid mcp payload", "device", h.deviceID, "error", err)
		return
	}
	slog.Info("voicebox: mcp message received", "device", h.deviceID, "payload_size", len(msg.Payload))
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
