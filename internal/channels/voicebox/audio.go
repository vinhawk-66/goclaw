package voicebox

import "sync"

const defaultAudioBufferMaxBytes = 2 * 1024 * 1024

// AudioBuffer accumulates uplink audio bytes for one listen window.
type AudioBuffer struct {
	mu       sync.Mutex
	data     []byte
	maxBytes int
}

func NewAudioBuffer(maxBytes int) *AudioBuffer {
	if maxBytes <= 0 {
		maxBytes = defaultAudioBufferMaxBytes
	}
	return &AudioBuffer{maxBytes: maxBytes}
}

func (b *AudioBuffer) Append(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(chunk) >= b.maxBytes {
		b.data = append(b.data[:0], chunk[len(chunk)-b.maxBytes:]...)
		return
	}

	next := len(b.data) + len(chunk)
	if next > b.maxBytes {
		over := next - b.maxBytes
		if over > len(b.data) {
			over = len(b.data)
		}
		b.data = append([]byte(nil), b.data[over:]...)
	}
	b.data = append(b.data, chunk...)
}

func (b *AudioBuffer) Reset() {
	b.mu.Lock()
	b.data = b.data[:0]
	b.mu.Unlock()
}

func (b *AudioBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, len(b.data))
	copy(out, b.data)
	return out
}
