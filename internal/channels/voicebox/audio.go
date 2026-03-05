package voicebox

import "sync"

const defaultAudioBufferMaxBytes = 2 * 1024 * 1024

// AudioBuffer accumulates uplink Opus packets for one listen window.
// Tracks individual packets (for OGG muxing) and total byte size (for cap).
type AudioBuffer struct {
	mu       sync.Mutex
	packets  [][]byte
	totalLen int
	maxBytes int
}

func NewAudioBuffer(maxBytes int) *AudioBuffer {
	if maxBytes <= 0 {
		maxBytes = defaultAudioBufferMaxBytes
	}
	return &AudioBuffer{maxBytes: maxBytes}
}

// Append adds one Opus packet to the buffer.
func (b *AudioBuffer) Append(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	// Evict oldest packets until under limit.
	for b.totalLen+len(chunk) > b.maxBytes && len(b.packets) > 0 {
		b.totalLen -= len(b.packets[0])
		b.packets = b.packets[1:]
	}

	pkt := make([]byte, len(chunk))
	copy(pkt, chunk)
	b.packets = append(b.packets, pkt)
	b.totalLen += len(pkt)
}

func (b *AudioBuffer) Reset() {
	b.mu.Lock()
	b.packets = b.packets[:0]
	b.totalLen = 0
	b.mu.Unlock()
}

// Packets returns a copy of accumulated Opus packets.
func (b *AudioBuffer) Packets() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([][]byte, len(b.packets))
	for i, p := range b.packets {
		cp := make([]byte, len(p))
		copy(cp, p)
		out[i] = cp
	}
	return out
}

// Bytes returns all packets concatenated (for backward compatibility).
func (b *AudioBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, 0, b.totalLen)
	for _, p := range b.packets {
		out = append(out, p...)
	}
	return out
}
