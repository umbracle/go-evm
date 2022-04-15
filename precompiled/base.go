package precompiled

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ethereum/evmc/v10/bindings/go/evmc"
	"github.com/umbracle/ethgo/wallet"
	"golang.org/x/crypto/ripemd160"
)

type Ecrecover struct {
	Precompiled
}

func (e *Ecrecover) Gas(input []byte, rev evmc.Revision) uint64 {
	return 3000
}

func (e *Ecrecover) Run(input []byte) ([]byte, error) {
	input, _ = e.get(input, 128)

	// recover the value v. Expect all zeros except the last byte
	for i := 32; i < 63; i++ {
		if input[i] != 0 {
			return nil, nil
		}
	}
	v := input[63] - 27
	if !validateSignatureValues(v, input[64:96], input[96:128]) {
		return nil, nil
	}

	addr, err := wallet.Ecrecover(input[:32], append(input[64:128], v))
	if err != nil {
		return nil, nil
	}
	dst := e.leftPad(addr.Bytes(), 32)
	return dst, nil
}

type Identity struct {
}

func (i *Identity) Gas(input []byte, rev evmc.Revision) uint64 {
	return baseGasCalc(input, 15, 3)
}

func (i *Identity) Run(in []byte) ([]byte, error) {
	return in, nil
}

type Sha256h struct {
}

func (s *Sha256h) Gas(input []byte, rev evmc.Revision) uint64 {
	return baseGasCalc(input, 60, 12)
}

func (s *Sha256h) Run(input []byte) ([]byte, error) {
	h := sha256.Sum256(input)
	return h[:], nil
}

type Ripemd160h struct {
	Precompiled
}

func (r *Ripemd160h) Gas(input []byte, rev evmc.Revision) uint64 {
	return baseGasCalc(input, 600, 120)
}

func (r *Ripemd160h) Run(input []byte) ([]byte, error) {
	ripemd := ripemd160.New()
	ripemd.Write(input)
	res := ripemd.Sum(nil)
	return r.leftPad(res, 32), nil
}

func baseGasCalc(input []byte, base, word uint64) uint64 {
	return base + uint64(len(input)+31)/32*word
}

// DecodeHex converts a hex string to a byte array
func DecodeHex(str string) ([]byte, error) {
	str = strings.TrimPrefix(str, "0x")

	return hex.DecodeString(str)
}

// MustDecodeHex type-checks and converts a hex string to a byte array
func MustDecodeHex(str string) []byte {
	buf, err := DecodeHex(str)
	if err != nil {
		panic(fmt.Errorf("could not decode hex: %v", err))
	}
	return buf
}

var (
	secp256k1N = MustDecodeHex("0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141")
	one        = []byte{0x01}
)

func trimLeftZeros(b []byte) []byte {
	i := 0
	for i = range b {
		if b[i] != 0 {
			break
		}
	}
	return b[i:]
}

// ValidateSignatureValues checks if the signature values are correct
func validateSignatureValues(v byte, r, s []byte) bool {
	// TODO: ECDSA malleability
	if v > 1 {
		return false
	}

	r = trimLeftZeros(r)
	if bytes.Compare(r, secp256k1N) >= 0 || bytes.Compare(r, one) < 0 {
		return false
	}

	s = trimLeftZeros(s)
	if bytes.Compare(s, secp256k1N) >= 0 || bytes.Compare(s, one) < 0 {
		return false
	}
	return true
}
