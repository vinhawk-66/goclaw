package voicebox

import "fmt"

// oggPageHeaderSize is the fixed portion of an OGG page header (before segment table).
const oggPageHeaderSize = 27

// ExtractOpusPackets parses an OGG/Opus stream and returns the raw Opus audio
// packets, skipping the OpusHead and OpusTags metadata packets (first 2 packets).
// ESP32 xiaozhi firmware expects raw Opus frames, not OGG container.
func ExtractOpusPackets(data []byte) ([][]byte, error) {
	var packets [][]byte
	offset := 0
	packetIndex := 0
	var packetBuf []byte // persists across pages for spanning packets

	for offset < len(data) {
		if offset+oggPageHeaderSize > len(data) {
			break
		}

		// Verify OGG capture pattern.
		if data[offset] != 'O' || data[offset+1] != 'g' || data[offset+2] != 'g' || data[offset+3] != 'S' {
			return nil, fmt.Errorf("ogg: invalid capture pattern at offset %d", offset)
		}

		numSegments := int(data[offset+26])
		segTableEnd := offset + oggPageHeaderSize + numSegments
		if segTableEnd > len(data) {
			return nil, fmt.Errorf("ogg: segment table overflow at offset %d", offset)
		}

		segTable := data[offset+oggPageHeaderSize : segTableEnd]
		pos := segTableEnd

		// Reconstruct packets from segments.
		// A segment of 255 bytes means continuation; < 255 means end of packet.
		for _, segLen := range segTable {
			segSize := int(segLen)
			if pos+segSize > len(data) {
				return nil, fmt.Errorf("ogg: segment data overflow at offset %d", pos)
			}
			packetBuf = append(packetBuf, data[pos:pos+segSize]...)
			pos += segSize

			if segSize < 255 {
				// Skip OpusHead (packet 0) and OpusTags (packet 1).
				if packetIndex >= 2 && len(packetBuf) > 0 {
					pkt := make([]byte, len(packetBuf))
					copy(pkt, packetBuf)
					packets = append(packets, pkt)
				}
				packetIndex++
				packetBuf = packetBuf[:0]
			}
		}

		offset = pos
	}

	return packets, nil
}
