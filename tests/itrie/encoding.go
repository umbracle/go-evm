package itrie

import (
	"bytes"
)

func hexToCompact(hex []byte) []byte {
	terminator := 0x0
	if hex[len(hex)-1] == 16 {
		terminator = 0x1
	}

	if terminator == 1 {
		hex = hex[:len(hex)-1]
	}

	oddlen := len(hex) % 2
	flags := 2*terminator + oddlen
	if oddlen != 0 {
		hex = append([]byte{byte(flags)}, hex...)
	} else {
		hex = append([]byte{byte(flags), 0}, hex...)
	}

	var buff bytes.Buffer
	for i := 0; i < len(hex); i += 2 {
		buff.WriteByte(byte(16*hex[i] + hex[i+1]))
	}
	return buff.Bytes()
}

func bytesToPath(str []byte) []byte {
	size := len(str)*2 + 1
	path := make([]byte, size)

	j := 0
	for i := 0; i < len(str); i, j = i+1, j+2 {
		b := str[i]
		path[j] = (b >> 4) & 0x0f
		path[j+1] = (b & 0x0f)
	}
	path[j] = 16
	return path
}
