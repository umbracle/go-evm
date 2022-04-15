package state

import (
	"github.com/ethereum/evmc/v10/bindings/go/evmc"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/fastrlp"
)

// createAddress creates an Ethereum address.
func createAddress(addr evmc.Address, nonce uint64) (res evmc.Address) {
	arena := fastrlp.Arena{}

	v := arena.NewArray()
	v.Set(arena.NewBytes(addr[:]))
	v.Set(arena.NewUint(nonce))

	dst := v.MarshalTo(nil)
	dst = ethgo.Keccak256(dst)

	copy(res[:], dst[12:])
	return
}

var create2Prefix = []byte{0xff}

// createAddress2 creates an Ethereum address following the CREATE2 Opcode.
func createAddress2(addr evmc.Address, salt [32]byte, inithash []byte) (res evmc.Address) {
	hash := ethgo.Keccak256(create2Prefix, addr[:], salt[:], ethgo.Keccak256(inithash))
	copy(res[:], hash[12:])
	return
}
