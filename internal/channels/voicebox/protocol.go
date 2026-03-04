package voicebox

import (
	"encoding/binary"
	"fmt"
)

const (
	FrameTypeAudio = 0
	FrameTypeJSON  = 1

	v2HeaderSize   = 16
	v3HeaderSize   = 4
	mqttHeaderSize = 16
)

// ParsedFrame is the canonical parsed binary frame payload.
type ParsedFrame struct {
	Type      int
	Timestamp uint32
	Payload   []byte
}

// ParseBinaryFrame extracts the payload for the negotiated protocol version.
func ParseBinaryFrame(data []byte, version int) (ParsedFrame, error) {
	switch version {
	case 2:
		return parseV2(data)
	case 3:
		return parseV3(data)
	default:
		return ParsedFrame{Type: FrameTypeAudio, Payload: data}, nil
	}
}

func parseV2(data []byte) (ParsedFrame, error) {
	if len(data) < v2HeaderSize {
		return ParsedFrame{}, fmt.Errorf("v2 frame too short: %d", len(data))
	}
	frameType := binary.BigEndian.Uint16(data[2:4])
	timestamp := binary.BigEndian.Uint32(data[8:12])
	payloadSize := binary.BigEndian.Uint32(data[12:16])
	end := v2HeaderSize + int(payloadSize)
	if end > len(data) {
		end = len(data)
	}
	return ParsedFrame{Type: int(frameType), Timestamp: timestamp, Payload: data[v2HeaderSize:end]}, nil
}

func parseV3(data []byte) (ParsedFrame, error) {
	if len(data) < v3HeaderSize {
		return ParsedFrame{}, fmt.Errorf("v3 frame too short: %d", len(data))
	}
	frameType := int(data[0])
	payloadSize := int(binary.BigEndian.Uint16(data[2:4]))
	end := v3HeaderSize + payloadSize
	if end > len(data) {
		end = len(data)
	}
	return ParsedFrame{Type: frameType, Payload: data[v3HeaderSize:end]}, nil
}

// BuildBinaryFrame wraps payload using the negotiated protocol version.
func BuildBinaryFrame(payload []byte, version int, timestamp uint32) []byte {
	switch version {
	case 2:
		return buildV2(payload, timestamp)
	case 3:
		return buildV3(payload)
	default:
		return payload
	}
}

func buildV2(payload []byte, timestamp uint32) []byte {
	buf := make([]byte, v2HeaderSize+len(payload))
	binary.BigEndian.PutUint16(buf[0:2], 2)
	binary.BigEndian.PutUint16(buf[2:4], FrameTypeAudio)
	binary.BigEndian.PutUint32(buf[4:8], 0)
	binary.BigEndian.PutUint32(buf[8:12], timestamp)
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(payload)))
	copy(buf[v2HeaderSize:], payload)
	return buf
}

func buildV3(payload []byte) []byte {
	buf := make([]byte, v3HeaderSize+len(payload))
	buf[0] = FrameTypeAudio
	buf[1] = 0
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(payload)))
	copy(buf[v3HeaderSize:], payload)
	return buf
}

// StripMQTTHeader removes the 16-byte envelope used by MQTT gateway mode.
func StripMQTTHeader(data []byte) []byte {
	if len(data) <= mqttHeaderSize {
		return data
	}
	return data[mqttHeaderSize:]
}
