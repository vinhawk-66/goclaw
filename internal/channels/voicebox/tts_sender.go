package voicebox

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/nextlevelbuilder/goclaw/internal/tts"
)

const opusChunkSizeBytes = 320

type ttsSynthesizer interface {
	Synthesize(ctx context.Context, text string, opts tts.Options) (*tts.SynthResult, error)
}

// TTSSender streams assistant output as protocol-compliant events and audio frames.
type TTSSender struct {
	sessionID       string
	protocolVersion int
	sendJSON        func(any) error
	sendBinary      func([]byte) error
	synth           ttsSynthesizer

	mu         sync.Mutex
	cancel     context.CancelFunc
	timestamp  uint32
}

func NewTTSSender(sessionID string, protocolVersion int, sendJSON func(any) error, sendBinary func([]byte) error, synth ttsSynthesizer) *TTSSender {
	return &TTSSender{
		sessionID:       sessionID,
		protocolVersion: protocolVersion,
		sendJSON:        sendJSON,
		sendBinary:      sendBinary,
		synth:           synth,
	}
}

func (s *TTSSender) SendResponse(parent context.Context, text, emotion string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if parent == nil {
		parent = context.Background()
	}

	ctx, cancel := context.WithCancel(parent)
	s.setCancel(cancel)
	defer func() {
		s.clearCancel()
		cancel()
	}()

	if emotion == "" {
		emotion = "neutral"
	}
	if err := s.sendJSON(LLMMessage{SessionID: s.sessionID, Type: "llm", Emotion: emotion}); err != nil {
		return err
	}
	if err := s.sendJSON(TTSMessage{SessionID: s.sessionID, Type: "tts", State: "start"}); err != nil {
		return err
	}
	defer func() {
		_ = s.sendJSON(TTSMessage{SessionID: s.sessionID, Type: "tts", State: "stop"})
	}()

	for _, sentence := range splitSentences(text) {
		if sentence == "" {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := s.sendJSON(TTSMessage{SessionID: s.sessionID, Type: "tts", State: "sentence_start", Text: sentence}); err != nil {
			return err
		}
		if s.synth == nil {
			continue
		}

		result, err := s.synth.Synthesize(ctx, sentence, tts.Options{Format: "opus"})
		if err != nil {
			slog.Warn("voicebox: tts synthesis failed", "error", err)
			continue
		}
		if err := s.sendAudio(result.Audio); err != nil {
			return err
		}
	}
	return nil
}

func (s *TTSSender) Cancel() {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *TTSSender) sendAudio(audio []byte) error {
	if len(audio) == 0 {
		return nil
	}
	if s.sendBinary == nil {
		return nil
	}

	// OGG container detected — demux to extract raw Opus packets.
	// All TTS providers (OpenAI, ElevenLabs, Edge) return OGG-wrapped Opus,
	// but ESP32 xiaozhi firmware expects raw Opus frames.
	if len(audio) >= 4 && string(audio[:4]) == "OggS" {
		packets, err := ExtractOpusPackets(audio)
		if err != nil {
			slog.Warn("voicebox: ogg demux failed, falling back to raw chunking", "error", err)
		} else {
			for _, pkt := range packets {
				frame := BuildBinaryFrame(pkt, s.protocolVersion, s.nextTimestamp())
				if err := s.sendBinary(frame); err != nil {
					return err
				}
			}
			return nil
		}
	}

	// Raw audio fallback: chunk at fixed boundaries.
	for start := 0; start < len(audio); start += opusChunkSizeBytes {
		end := start + opusChunkSizeBytes
		if end > len(audio) {
			end = len(audio)
		}
		frame := BuildBinaryFrame(audio[start:end], s.protocolVersion, s.nextTimestamp())
		if err := s.sendBinary(frame); err != nil {
			return err
		}
	}
	return nil
}

func (s *TTSSender) nextTimestamp() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timestamp += 60
	return s.timestamp
}

func (s *TTSSender) setCancel(cancel context.CancelFunc) {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = cancel
	s.mu.Unlock()
}

func (s *TTSSender) clearCancel() {
	s.mu.Lock()
	s.cancel = nil
	s.mu.Unlock()
}

func splitSentences(text string) []string {
	parts := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '.', '!', '?', '\n', '。', '！', '？':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{text}
	}
	return out
}
