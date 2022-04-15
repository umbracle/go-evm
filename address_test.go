package state

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ethereum/evmc/v10/bindings/go/evmc"
	"github.com/stretchr/testify/assert"
)

func TestCreate2(t *testing.T) {
	cases := []struct {
		address  string
		salt     string
		initCode string
		result   string
	}{
		{
			"0000000000000000000000000000000000000000",
			"0000000000000000000000000000000000000000000000000000000000000000",
			"00",
			"4D1A2e2bB4F88F0250f26Ffff098B0b30B26BF38",
		},
		{
			"deadbeef00000000000000000000000000000000",
			"0000000000000000000000000000000000000000000000000000000000000000",
			"00",
			"B928f69Bb1D91Cd65274e3c79d8986362984fDA3",
		},
		{
			"deadbeef00000000000000000000000000000000",
			"000000000000000000000000feed000000000000000000000000000000000000",
			"00",
			"D04116cDd17beBE565EB2422F2497E06cC1C9833",
		},
		{
			"0000000000000000000000000000000000000000",
			"0000000000000000000000000000000000000000000000000000000000000000",
			"deadbeef",
			"70f2b2914A2a4b783FaEFb75f459A580616Fcb5e",
		},
		{
			"00000000000000000000000000000000deadbeef",
			"00000000000000000000000000000000000000000000000000000000cafebabe",
			"deadbeef",
			"60f3f640a8508fC6a86d45DF051962668E1e8AC7",
		},
		{
			"00000000000000000000000000000000deadbeef",
			"00000000000000000000000000000000000000000000000000000000cafebabe",
			"deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			"1d8bfDC5D46DC4f61D6b6115972536eBE6A8854C",
		},
		{
			"0000000000000000000000000000000000000000",
			"0000000000000000000000000000000000000000000000000000000000000000",
			"",
			"E33C0C7F7df4809055C3ebA6c09CFe4BaF1BD9e0",
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			addrRaw, _ := hex.DecodeString(c.address)
			var address evmc.Address
			copy(address[:], addrRaw)

			initCode, _ := hex.DecodeString(c.initCode)

			saltRaw, _ := hex.DecodeString(c.salt)
			var salt [32]byte
			copy(salt[:], saltRaw)

			res := createAddress2(address, salt, initCode)
			assert.Equal(t, strings.ToLower(c.result), hex.EncodeToString(res[:]))
		})
	}
}
