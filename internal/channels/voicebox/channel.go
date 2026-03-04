package voicebox

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const (
	defaultDMPolicy     = "pairing"
	pairingDebounceTime = 60 * time.Second
)

// ChannelConfig is the runtime configuration for a voicebox channel instance.
type ChannelConfig struct {
	DMPolicy   string
	AllowFrom  []string
}

// Channel implements channels.Channel + channels.WebhookChannel for voice devices.
type Channel struct {
	*channels.BaseChannel
	upgrader       websocket.Upgrader
	pairingService store.PairingStore
	tokenAuth      *TokenAuth
	sttProxy       *STTProxy
	config         ChannelConfig

	ctx    context.Context
	cancel context.CancelFunc

	handlers        sync.Map // deviceID -> *Handler
	sessionToDevice sync.Map // sessionID -> deviceID
	pairingDebounce sync.Map // senderID -> time.Time
}

var (
	activeChannelMu sync.RWMutex
	activeChannel   *Channel
)

func New(cfg ChannelConfig, msgBus *bus.MessageBus, pairingSvc store.PairingStore, tokenAuth *TokenAuth, sttProxy *STTProxy) *Channel {
	base := channels.NewBaseChannel("voicebox", msgBus, cfg.AllowFrom)
	base.ValidatePolicy(cfg.DMPolicy, "")
	if cfg.DMPolicy == "" {
		cfg.DMPolicy = defaultDMPolicy
	}

	return &Channel{
		BaseChannel:     base,
		pairingService:  pairingSvc,
		tokenAuth:       tokenAuth,
		sttProxy:        sttProxy,
		config:          cfg,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin:     allowVoiceboxOrigin,
		},
	}
}

func (c *Channel) Start(ctx context.Context) error {
	if c.IsRunning() {
		return nil
	}
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.SetRunning(true)
	setActiveChannel(c)
	slog.Info("voicebox channel started", "name", c.Name())
	return nil
}

func (c *Channel) Stop(_ context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	c.handlers.Range(func(_, value any) bool {
		if h, ok := value.(*Handler); ok {
			h.Close()
		}
		return true
	})
	clearActiveChannel(c)
	c.SetRunning(false)
	slog.Info("voicebox channel stopped", "name", c.Name())
	return nil
}

func (c *Channel) WebhookHandler() (string, http.Handler) {
	return "/voicebox/v1/", http.HandlerFunc(serveActiveVoiceboxWS)
}

func (c *Channel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	h, ok := c.handlerForChat(msg.ChatID)
	if !ok {
		return fmt.Errorf("voicebox device not connected: %s", msg.ChatID)
	}
	return h.SendAssistantReply(ctx, msg.Content, msg.Metadata)
}

func (c *Channel) handlerForChat(chatID string) (*Handler, bool) {
	if chatID == "" {
		return nil, false
	}
	v, ok := c.handlers.Load(chatID)
	if !ok {
		return nil, false
	}
	h, ok := v.(*Handler)
	return h, ok
}

func (c *Channel) registerHandler(deviceID string, h *Handler) {
	if prev, ok := c.handlers.Load(deviceID); ok {
		if old, ok := prev.(*Handler); ok && old != h {
			old.Close()
		}
	}
	c.handlers.Store(deviceID, h)
}

func (c *Channel) bindSession(sessionID, deviceID string) {
	if sessionID != "" && deviceID != "" {
		c.sessionToDevice.Store(sessionID, deviceID)
	}
}

func (c *Channel) unregisterHandler(deviceID, sessionID string, current *Handler) {
	if deviceID != "" {
		if loaded, ok := c.handlers.Load(deviceID); ok {
			if h, ok := loaded.(*Handler); ok && h == current {
				c.handlers.Delete(deviceID)
			}
		}
	}
	if sessionID != "" {
		c.sessionToDevice.Delete(sessionID)
	}
}

func serveActiveVoiceboxWS(w http.ResponseWriter, r *http.Request) {
	ch := getActiveChannel()
	if ch == nil {
		http.Error(w, "voicebox channel unavailable", http.StatusServiceUnavailable)
		return
	}
	ch.serveWS(w, r)
}

func setActiveChannel(c *Channel) {
	activeChannelMu.Lock()
	activeChannel = c
	activeChannelMu.Unlock()
}

func clearActiveChannel(c *Channel) {
	activeChannelMu.Lock()
	if activeChannel == c {
		activeChannel = nil
	}
	activeChannelMu.Unlock()
}

func getActiveChannel() *Channel {
	activeChannelMu.RLock()
	defer activeChannelMu.RUnlock()
	return activeChannel
}

func allowVoiceboxOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}
