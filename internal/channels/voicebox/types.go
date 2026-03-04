package voicebox

import "encoding/json"

// ClientHello is sent by device immediately after WS connect.
type ClientHello struct {
	Type        string       `json:"type"`
	Version     int          `json:"version,omitempty"`
	Transport   string       `json:"transport"`
	AudioParams *AudioParams `json:"audio_params,omitempty"`
	Features    *Features    `json:"features,omitempty"`
}

// AudioParams describes codec parameters for client/server hello.
type AudioParams struct {
	Format        string `json:"format,omitempty"`
	SampleRate    int    `json:"sample_rate,omitempty"`
	Channels      int    `json:"channels,omitempty"`
	FrameDuration int    `json:"frame_duration,omitempty"`
}

// Features describes optional client capabilities declared during hello.
type Features struct {
	MCP bool `json:"mcp,omitempty"`
	AEC bool `json:"aec,omitempty"`
}

// ServerHello is sent by server in response to client hello.
type ServerHello struct {
	Type        string       `json:"type"`
	Transport   string       `json:"transport"`
	SessionID   string       `json:"session_id,omitempty"`
	AudioParams *AudioParams `json:"audio_params,omitempty"`
}

// Message is the minimal common envelope used for incoming text messages.
type Message struct {
	SessionID string `json:"session_id,omitempty"`
	Type      string `json:"type"`
}

// ListenMessage controls voice capture state on device.
type ListenMessage struct {
	SessionID string `json:"session_id,omitempty"`
	Type      string `json:"type"`
	State     string `json:"state"`
	Mode      string `json:"mode,omitempty"`
	Text      string `json:"text,omitempty"`
}

// AbortMessage asks server to stop current playback.
type AbortMessage struct {
	SessionID string `json:"session_id,omitempty"`
	Type      string `json:"type"`
	Reason    string `json:"reason,omitempty"`
}

// TTSMessage describes outbound TTS playback events.
type TTSMessage struct {
	SessionID string `json:"session_id,omitempty"`
	Type      string `json:"type"`
	State     string `json:"state"`
	Text      string `json:"text,omitempty"`
}

// STTMessage sends ASR text back to the device.
type STTMessage struct {
	SessionID string `json:"session_id,omitempty"`
	Type      string `json:"type"`
	Text      string `json:"text"`
}

// LLMMessage carries optional expression metadata.
type LLMMessage struct {
	SessionID string `json:"session_id,omitempty"`
	Type      string `json:"type"`
	Emotion   string `json:"emotion"`
}

// MCPMessage wraps a JSON-RPC payload.
type MCPMessage struct {
	SessionID string          `json:"session_id,omitempty"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

// SystemMessage sends command-level actions to a device.
type SystemMessage struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// AlertMessage sends human-readable warning/error notices.
type AlertMessage struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Emotion string `json:"emotion,omitempty"`
}
