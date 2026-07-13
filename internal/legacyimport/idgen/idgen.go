package idgen

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"
)

func UUIDv7(unixMillis int64, key string) string {
	h := sha256.Sum256([]byte(key))

	var b [16]byte
	// 48-bit big-endian timestamp in bytes 0..5.
	ts := uint64(unixMillis) & 0xFFFFFFFFFFFF
	b[0] = byte(ts >> 40)
	b[1] = byte(ts >> 32)
	b[2] = byte(ts >> 24)
	b[3] = byte(ts >> 16)
	b[4] = byte(ts >> 8)
	b[5] = byte(ts)
	// Bytes 6..15 from the hash.
	copy(b[6:], h[:10])
	// Version 7 (high nibble of byte 6).
	b[6] = (b[6] & 0x0F) | 0x70
	// Variant 10 (two high bits of byte 8).
	b[8] = (b[8] & 0x3F) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16],
	)
}

func MillisFromZ(s string, fallback int64) int64 {
	t, err := time.Parse("2006-01-02T15:04:05Z", s)
	if err != nil {
		return fallback
	}
	return t.UnixMilli()
}
