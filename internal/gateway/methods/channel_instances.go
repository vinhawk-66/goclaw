package methods

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// ChannelInstancesMethods handles channel instance CRUD via WebSocket RPC (managed mode).
type ChannelInstancesMethods struct {
	store  store.ChannelInstanceStore
	msgBus *bus.MessageBus
}

// NewChannelInstancesMethods creates a new handler for channel instance management.
func NewChannelInstancesMethods(s store.ChannelInstanceStore, msgBus *bus.MessageBus) *ChannelInstancesMethods {
	return &ChannelInstancesMethods{store: s, msgBus: msgBus}
}

// Register registers all channel instance RPC methods.
func (m *ChannelInstancesMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodChannelInstancesList, m.handleList)
	router.Register(protocol.MethodChannelInstancesGet, m.handleGet)
	router.Register(protocol.MethodChannelInstancesCreate, m.handleCreate)
	router.Register(protocol.MethodChannelInstancesUpdate, m.handleUpdate)
	router.Register(protocol.MethodChannelInstancesDelete, m.handleDelete)
}

func (m *ChannelInstancesMethods) emitCacheInvalidate() {
	if m.msgBus == nil {
		return
	}
	m.msgBus.Broadcast(bus.Event{
		Name:    protocol.EventCacheInvalidate,
		Payload: bus.CacheInvalidatePayload{Kind: bus.CacheKindChannelInstances},
	})
}

func (m *ChannelInstancesMethods) handleList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	instances, err := m.store.ListAll(ctx)
	if err != nil {
		slog.Error("channels.instances.list", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to list channel instances"))
		return
	}

	// Mask credentials in response — never expose secrets via WS.
	result := make([]map[string]interface{}, 0, len(instances))
	for _, inst := range instances {
		result = append(result, maskInstance(inst))
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{
		"instances": result,
	}))
}

func (m *ChannelInstancesMethods) handleGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params struct {
		ID string `json:"id"`
	}
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}

	id, err := uuid.Parse(params.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid instance ID"))
		return
	}

	inst, err := m.store.Get(ctx, id)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, "instance not found"))
		return
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, maskInstance(*inst)))
}

func (m *ChannelInstancesMethods) handleCreate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params struct {
		Name        string          `json:"name"`
		DisplayName string          `json:"display_name"`
		ChannelType string          `json:"channel_type"`
		AgentID     string          `json:"agent_id"`
		Credentials json.RawMessage `json:"credentials"`
		Config      json.RawMessage `json:"config"`
		Enabled     *bool           `json:"enabled"`
	}
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}

	if params.Name == "" || params.ChannelType == "" || params.AgentID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "name, channel_type, and agent_id are required"))
		return
	}

	if !isValidChannelType(params.ChannelType) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid channel_type"))
		return
	}

	agentID, err := uuid.Parse(params.AgentID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid agent_id"))
		return
	}

	enabled := true
	if params.Enabled != nil {
		enabled = *params.Enabled
	}
	if params.ChannelType == "voicebox" && enabled {
		if err := ensureSingleEnabledVoicebox(ctx, m.store, uuid.Nil); err != nil {
			if errors.Is(err, errVoiceboxAlreadyEnabled) {
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "only one enabled voicebox instance is supported"))
				return
			}
			slog.Error("channels.instances.create", "error", err)
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to validate voicebox instances"))
			return
		}
	}
	if params.ChannelType == "voicebox" {
		if err := validateVoiceboxAuthRaw(params.Config, params.Credentials); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, err.Error()))
			return
		}
	}

	inst := &store.ChannelInstanceData{
		Name:        params.Name,
		DisplayName: params.DisplayName,
		ChannelType: params.ChannelType,
		AgentID:     agentID,
		Credentials: params.Credentials,
		Config:      params.Config,
		Enabled:     enabled,
	}

	if err := m.store.Create(ctx, inst); err != nil {
		slog.Error("channels.instances.create", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to create instance: "+err.Error()))
		return
	}

	m.emitCacheInvalidate()
	client.SendResponse(protocol.NewOKResponse(req.ID, maskInstance(*inst)))
}

func (m *ChannelInstancesMethods) handleUpdate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params struct {
		ID      string          `json:"id"`
		Updates json.RawMessage `json:"updates"`
	}
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}

	id, err := uuid.Parse(params.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid instance ID"))
		return
	}

	var updates map[string]interface{}
	if err := json.Unmarshal(params.Updates, &updates); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid updates"))
		return
	}

	current, err := m.store.Get(ctx, id)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, "instance not found"))
		return
	}

	nextChannelType := current.ChannelType
	if v, ok := updates["channel_type"].(string); ok && v != "" {
		nextChannelType = v
	}
	if !isValidChannelType(nextChannelType) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid channel_type"))
		return
	}
	nextEnabled := current.Enabled
	if v, ok := updates["enabled"].(bool); ok {
		nextEnabled = v
	}

	cfgRaw := []byte(current.Config)
	if raw, exists, err := rawJSONFromUpdateValue(updates, "config"); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid config update"))
		return
	} else if exists {
		cfgRaw = raw
	}

	credsRaw := current.Credentials
	if raw, exists, err := rawJSONFromUpdateValue(updates, "credentials"); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid credentials update"))
		return
	} else if exists {
		credsRaw = raw
	}

	if nextChannelType == "voicebox" && nextEnabled {
		if err := ensureSingleEnabledVoicebox(ctx, m.store, id); err != nil {
			if errors.Is(err, errVoiceboxAlreadyEnabled) {
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "only one enabled voicebox instance is supported"))
				return
			}
			slog.Error("channels.instances.update", "error", err)
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to validate voicebox instances"))
			return
		}
	}
	if nextChannelType == "voicebox" {
		if err := validateVoiceboxAuthRaw(cfgRaw, credsRaw); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, err.Error()))
			return
		}
	}

	if err := m.store.Update(ctx, id, updates); err != nil {
		slog.Error("channels.instances.update", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to update instance: "+err.Error()))
		return
	}

	m.emitCacheInvalidate()
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{"status": "updated"}))
}

func (m *ChannelInstancesMethods) handleDelete(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var params struct {
		ID string `json:"id"`
	}
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}

	id, err := uuid.Parse(params.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid instance ID"))
		return
	}

	// Look up instance to check if it's a default (seeded) instance.
	inst, err := m.store.Get(ctx, id)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "instance not found"))
		return
	}
	if store.IsDefaultChannelInstance(inst.Name) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "cannot delete default channel instance"))
		return
	}

	if err := m.store.Delete(ctx, id); err != nil {
		slog.Error("channels.instances.delete", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "failed to delete instance: "+err.Error()))
		return
	}

	m.emitCacheInvalidate()
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]interface{}{"status": "deleted"}))
}

// maskInstance returns a map representation with credentials masked.
func maskInstance(inst store.ChannelInstanceData) map[string]interface{} {
	result := map[string]interface{}{
		"id":           inst.ID,
		"name":         inst.Name,
		"display_name": inst.DisplayName,
		"channel_type": inst.ChannelType,
		"agent_id":     inst.AgentID,
		"config":       inst.Config,
		"enabled":      inst.Enabled,
		"is_default":       store.IsDefaultChannelInstance(inst.Name),
		"has_credentials":  len(inst.Credentials) > 0,
		"created_by":       inst.CreatedBy,
		"created_at":       inst.CreatedAt,
		"updated_at":       inst.UpdatedAt,
	}

	// Mask credentials: show keys with "***" values
	if len(inst.Credentials) > 0 {
		var raw map[string]interface{}
		if json.Unmarshal(inst.Credentials, &raw) == nil {
			masked := make(map[string]interface{}, len(raw))
			for k := range raw {
				masked[k] = "***"
			}
			result["credentials"] = masked
		} else {
			result["credentials"] = map[string]string{}
		}
	} else {
		result["credentials"] = map[string]string{}
	}

	return result
}

// isValidChannelType checks if the channel type is supported.
func isValidChannelType(ct string) bool {
	switch ct {
	case "telegram", "discord", "whatsapp", "zalo_oa", "zalo_personal", "feishu", "voicebox":
		return true
	}
	return false
}

func ensureSingleEnabledVoicebox(ctx context.Context, s store.ChannelInstanceStore, excludeID uuid.UUID) error {
	enabledInstances, err := s.ListEnabled(ctx)
	if err != nil {
		return err
	}
	for _, inst := range enabledInstances {
		if inst.ChannelType == "voicebox" && (excludeID == uuid.Nil || inst.ID != excludeID) {
			return errVoiceboxAlreadyEnabled
		}
	}
	return nil
}

func rawJSONFromUpdateValue(updates map[string]interface{}, key string) ([]byte, bool, error) {
	v, ok := updates[key]
	if !ok {
		return nil, false, nil
	}
	switch t := v.(type) {
	case json.RawMessage:
		return t, true, nil
	case []byte:
		return t, true, nil
	case string:
		return []byte(t), true, nil
	default:
		raw, err := json.Marshal(t)
		return raw, true, err
	}
}

func validateVoiceboxAuthRaw(cfgRaw, credsRaw []byte) error {
	var cfg struct {
		AuthMode string `json:"auth_mode,omitempty"`
	}
	if len(cfgRaw) > 0 {
		if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
			return fmt.Errorf("invalid voicebox config")
		}
	}
	switch cfg.AuthMode {
	case "", "open":
		return nil
	case "token":
		// continue
	default:
		return fmt.Errorf("invalid auth_mode: %s", cfg.AuthMode)
	}
	if cfg.AuthMode != "token" {
		return nil
	}

	var creds struct {
		SecretKey string `json:"secret_key,omitempty"`
	}
	if len(credsRaw) > 0 {
		if err := json.Unmarshal(credsRaw, &creds); err != nil {
			return fmt.Errorf("invalid voicebox credentials")
		}
	}
	if strings.TrimSpace(creds.SecretKey) == "" {
		return fmt.Errorf("secret_key is required when auth_mode=token")
	}
	return nil
}

var errVoiceboxAlreadyEnabled = errors.New("voicebox instance already enabled")
