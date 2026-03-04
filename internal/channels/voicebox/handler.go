package voicebox

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	helloTimeout    = 10 * time.Second
	idleTimeout     = 120 * time.Second
	writeTimeout    = 10 * time.Second
	maxMessageBytes = 2 << 20
)

// Handler manages one connected voicebox device.
type Handler struct {
	channel         *Channel
	conn            *websocket.Conn
	deviceID        string
	clientID        string
	protocolVersion int
	mqttGateway     bool

	sessionID  string
	session    *Session
	audio      *AudioBuffer
	listening  bool
	ttsSender  *TTSSender

	mu       sync.Mutex
	writeMu  sync.Mutex
	closeOnce sync.Once
}

func NewHandler(channel *Channel, conn *websocket.Conn, deviceID, clientID string, protocolVersion int, mqttGateway bool) *Handler {
	return &Handler{
		channel:         channel,
		conn:            conn,
		deviceID:        deviceID,
		clientID:        clientID,
		protocolVersion: protocolVersion,
		mqttGateway:     mqttGateway,
		audio:           NewAudioBuffer(0),
	}
}

func (h *Handler) Run(ctx context.Context) {
	defer func() {
		h.channel.unregisterHandler(h.deviceID, h.sessionID, h)
		h.Close()
	}()

	h.conn.SetReadLimit(maxMessageBytes)
	h.conn.SetPongHandler(func(string) error {
		_ = h.conn.SetReadDeadline(time.Now().Add(idleTimeout))
		return nil
	})
	_ = h.conn.SetReadDeadline(time.Now().Add(helloTimeout))

	hello, err := h.readClientHello()
	if err != nil {
		slog.Warn("voicebox: hello failed", "device", h.deviceID, "error", err)
		return
	}
	if hello.Version > 0 {
		h.protocolVersion = hello.Version
	}
	if h.protocolVersion < 1 || h.protocolVersion > 3 {
		h.protocolVersion = 1
	}

	h.session = NewSession(hello.Features)
	h.sessionID = uuid.NewString()
	h.channel.bindSession(h.sessionID, h.deviceID)
	h.ttsSender = NewTTSSender(h.sessionID, h.protocolVersion, h.writeJSON, h.writeBinary, currentTTSManager())

	if err := h.sendServerHello(); err != nil {
		slog.Warn("voicebox: send hello failed", "device", h.deviceID, "error", err)
		return
	}
	_ = h.conn.SetReadDeadline(time.Now().Add(idleTimeout))

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgType, data, err := h.conn.ReadMessage()
		if err != nil {
			if isTimeout(err) {
				slog.Info("voicebox: connection idle timeout", "device", h.deviceID)
				return
			}
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				slog.Debug("voicebox: read failed", "device", h.deviceID, "error", err)
			}
			return
		}
		_ = h.conn.SetReadDeadline(time.Now().Add(idleTimeout))

		switch msgType {
		case websocket.TextMessage:
			h.handleTextMessage(ctx, data)
		case websocket.BinaryMessage:
			h.handleBinaryMessage(ctx, data)
		}
	}
}

func (h *Handler) Close() {
	h.closeOnce.Do(func() {
		h.mu.Lock()
		sender := h.ttsSender
		h.mu.Unlock()
		if sender != nil {
			sender.Cancel()
		}
		_ = h.conn.Close()
	})
}

func (h *Handler) readClientHello() (*ClientHello, error) {
	msgType, data, err := h.conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if msgType != websocket.TextMessage {
		return nil, errors.New("hello must be text")
	}

	var hello ClientHello
	if err := json.Unmarshal(data, &hello); err != nil {
		return nil, err
	}
	if hello.Type != "hello" {
		return nil, errors.New("invalid hello type")
	}
	if hello.Transport != "websocket" {
		return nil, errors.New("invalid hello transport")
	}
	return &hello, nil
}

func (h *Handler) sendServerHello() error {
	return h.writeJSON(ServerHello{
		Type:      "hello",
		Transport: "websocket",
		SessionID: h.sessionID,
		AudioParams: &AudioParams{
			SampleRate:    24000,
			FrameDuration: 60,
		},
	})
}

func (h *Handler) writeJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	_ = h.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return h.conn.WriteMessage(websocket.TextMessage, b)
}

func (h *Handler) writeBinary(data []byte) error {
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	_ = h.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return h.conn.WriteMessage(websocket.BinaryMessage, data)
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
