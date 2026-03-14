package voicebox

import "testing"

func TestParseAndBuildV2(t *testing.T) {
	payload := []byte{1, 2, 3, 4, 5}
	frame := BuildBinaryFrame(payload, 2, 123)
	parsed, err := ParseBinaryFrame(frame, 2)
	if err != nil {
		t.Fatalf("parse v2 failed: %v", err)
	}
	if parsed.Type != FrameTypeAudio {
		t.Fatalf("unexpected frame type: %d", parsed.Type)
	}
	if parsed.Timestamp != 123 {
		t.Fatalf("unexpected timestamp: %d", parsed.Timestamp)
	}
	if len(parsed.Payload) != len(payload) {
		t.Fatalf("unexpected payload length: %d", len(parsed.Payload))
	}
}

func TestParseAndBuildV3(t *testing.T) {
	payload := []byte{9, 8, 7}
	frame := BuildBinaryFrame(payload, 3, 0)
	parsed, err := ParseBinaryFrame(frame, 3)
	if err != nil {
		t.Fatalf("parse v3 failed: %v", err)
	}
	if parsed.Type != FrameTypeAudio {
		t.Fatalf("unexpected frame type: %d", parsed.Type)
	}
	if len(parsed.Payload) != len(payload) {
		t.Fatalf("unexpected payload length: %d", len(parsed.Payload))
	}
}

func TestStripMQTTHeader(t *testing.T) {
	in := make([]byte, mqttHeaderSize+4)
	in[mqttHeaderSize] = 0xAA
	out := StripMQTTHeader(in)
	if len(out) != 4 || out[0] != 0xAA {
		t.Fatalf("unexpected stripped payload: %#v", out)
	}
}
