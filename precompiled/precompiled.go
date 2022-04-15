package precompiled

import (
	"encoding/binary"
)

// Precompiled is a helper runtime for precompiled smart contracts
type Precompiled struct {
}

var zeroPadding = make([]byte, 64)

func (p *Precompiled) leftPad(buf []byte, n int) []byte {
	// TODO, avoid buffer allocation
	l := len(buf)
	if l > n {
		return buf
	}

	tmp := make([]byte, n)
	copy(tmp[n-l:], buf)
	return tmp
}

func (p *Precompiled) get(input []byte, size int) ([]byte, []byte) {
	//p.buf = extendByteSlice(p.buf, size)
	buf := make([]byte, size)

	n := size
	if len(input) < n {
		n = len(input)
	}

	// copy the part from the input
	copy(buf[0:], input[:n])

	// copy empty values
	if n < size {
		rest := size - n
		if rest < 64 {
			copy(buf[n:], zeroPadding[0:size-n])
		} else {
			copy(buf[n:], make([]byte, rest))
		}
	}
	return buf, input[n:]
}

func (p *Precompiled) getUint64(input []byte) (uint64, []byte) {
	var buf []byte
	buf, input = p.get(input, 32)
	num := binary.BigEndian.Uint64(buf[24:32])
	return num, input
}
