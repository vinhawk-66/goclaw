package voicebox

import (
	"encoding/binary"
)

// WrapOpusInOGG packages raw Opus frames into a valid OGG/Opus container
// so that STT APIs (Whisper, Groq, etc.) can decode it.
//
// Parameters:
//   - packets: raw Opus audio frames (each frame = one Opus packet)
//   - sampleRate: input sample rate (e.g. 16000)
//   - channels: number of audio channels (typically 1)
//   - frameDurationMs: frame duration in milliseconds (e.g. 60)
func WrapOpusInOGG(packets [][]byte, sampleRate, channels, frameDurationMs int) []byte {
	if len(packets) == 0 {
		return nil
	}

	serialNo := uint32(0x47436C61) // "GCla" — arbitrary stream serial
	var buf []byte

	// Page 0: OpusHead header.
	opusHead := buildOpusHead(uint8(channels), uint16(0), uint32(sampleRate))
	buf = append(buf, buildOGGPage(serialNo, 0, 0, 0x02, [][]byte{opusHead})...) // BOS flag

	// Page 1: OpusTags header.
	opusTags := buildOpusTags()
	buf = append(buf, buildOGGPage(serialNo, 1, 0, 0x00, [][]byte{opusTags})...)

	// Audio pages: one Opus packet per page for simplicity.
	samplesPerFrame := uint64(sampleRate * frameDurationMs / 1000)
	var granule uint64

	for i, pkt := range packets {
		granule += samplesPerFrame
		flags := byte(0x00)
		if i == len(packets)-1 {
			flags = 0x04 // EOS flag on last page
		}
		page := buildOGGPage(serialNo, uint32(i+2), granule, flags, [][]byte{pkt})
		buf = append(buf, page...)
	}

	return buf
}

// buildOpusHead creates the 19-byte OpusHead packet per RFC 7845 §5.1.
func buildOpusHead(channels uint8, preSkip uint16, sampleRate uint32) []byte {
	head := make([]byte, 19)
	copy(head[0:8], "OpusHead")
	head[8] = 1          // version
	head[9] = channels   // channel count
	binary.LittleEndian.PutUint16(head[10:12], preSkip)
	binary.LittleEndian.PutUint32(head[12:16], sampleRate)
	binary.LittleEndian.PutUint16(head[16:18], 0) // output gain
	head[18] = 0 // channel mapping family
	return head
}

// buildOpusTags creates a minimal OpusTags packet per RFC 7845 §5.2.
func buildOpusTags() []byte {
	vendor := "goclaw"
	// 8 bytes "OpusTags" + 4 bytes vendor length + vendor + 4 bytes comment count
	tags := make([]byte, 8+4+len(vendor)+4)
	copy(tags[0:8], "OpusTags")
	binary.LittleEndian.PutUint32(tags[8:12], uint32(len(vendor)))
	copy(tags[12:12+len(vendor)], vendor)
	binary.LittleEndian.PutUint32(tags[12+len(vendor):], 0) // 0 user comments
	return tags
}

// buildOGGPage constructs one OGG page with the given segments.
func buildOGGPage(serialNo uint32, pageSeq uint32, granule uint64, flags byte, segments [][]byte) []byte {
	// Build segment table: each segment up to 255 bytes per lacing value.
	var segTable []byte
	for _, seg := range segments {
		n := len(seg)
		for n >= 255 {
			segTable = append(segTable, 255)
			n -= 255
		}
		segTable = append(segTable, byte(n))
	}

	headerSize := 27 + len(segTable)
	var payloadSize int
	for _, seg := range segments {
		payloadSize += len(seg)
	}

	page := make([]byte, headerSize+payloadSize)

	// Capture pattern.
	copy(page[0:4], "OggS")
	page[4] = 0    // version
	page[5] = flags // header type flags
	binary.LittleEndian.PutUint64(page[6:14], granule)
	binary.LittleEndian.PutUint32(page[14:18], serialNo)
	binary.LittleEndian.PutUint32(page[18:22], pageSeq)
	// CRC at [22:26] — set to 0 first, compute after.
	binary.LittleEndian.PutUint32(page[22:26], 0)
	page[26] = byte(len(segTable))
	copy(page[27:27+len(segTable)], segTable)

	// Copy payload.
	pos := headerSize
	for _, seg := range segments {
		copy(page[pos:], seg)
		pos += len(seg)
	}

	// Compute CRC-32 over entire page with CRC field set to 0.
	crc := oggCRC32(page)
	binary.LittleEndian.PutUint32(page[22:26], crc)

	return page
}

// oggCRC32 computes the OGG CRC-32 (polynomial 0x04C11DB7, non-reflected).
// Note: Go's crc32.MakeTable uses reflected form, but OGG uses a non-reflected
// variant. We implement it directly for correctness.
func oggCRC32(data []byte) uint32 {
	var crc uint32
	for _, b := range data {
		crc ^= uint32(b) << 24
		for i := 0; i < 8; i++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
