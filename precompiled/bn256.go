package precompiled

import (
	"fmt"
	"math/big"

	"github.com/ethereum/evmc/v10/bindings/go/evmc"
	bn256 "github.com/umbracle/go-eth-bn256"
)

type Bn256Add struct {
	Precompiled
}

func (b *Bn256Add) Gas(input []byte, rev evmc.Revision) uint64 {
	if rev >= evmc.Istanbul {
		return 150
	}
	return 500
}

func (b *Bn256Add) Run(input []byte) ([]byte, error) {
	var val []byte

	b1 := new(bn256.G1)
	b2 := new(bn256.G1)

	val, input = b.get(input, 64)
	if _, err := b1.Unmarshal(val); err != nil {
		return nil, err
	}

	val, _ = b.get(input, 64)
	if _, err := b2.Unmarshal(val); err != nil {
		return nil, err
	}

	c := new(bn256.G1)
	c.Add(b1, b2)
	return c.Marshal(), nil
}

type Bn256Mul struct {
	Precompiled
}

func (b *Bn256Mul) Gas(input []byte, rev evmc.Revision) uint64 {
	if rev >= evmc.Istanbul {
		return 6000
	}
	return 40000
}

func (b *Bn256Mul) Run(input []byte) ([]byte, error) {
	var v []byte

	b0 := new(bn256.G1)
	v, input = b.get(input, 64)
	if _, err := b0.Unmarshal(v); err != nil {
		return nil, err
	}

	v, _ = b.get(input, 32)
	k := new(big.Int).SetBytes(v)

	c := new(bn256.G1)
	c.ScalarMult(b0, k)
	return c.Marshal(), nil
}

var (
	falseBytes = make([]byte, 32)
	trueBytes  = make([]byte, 32)
)

func init() {
	trueBytes[31] = 1
}

type Bn256Pairing struct {
	Precompiled
}

func (b *Bn256Pairing) Gas(input []byte, rev evmc.Revision) uint64 {
	baseGas, pointGas := uint64(100000), uint64(80000)
	if rev >= evmc.Istanbul {
		baseGas, pointGas = 45000, 34000
	}
	return baseGas + pointGas*uint64(len(input)/192)
}

func (b *Bn256Pairing) Run(input []byte) ([]byte, error) {
	if len(input) == 0 {
		return trueBytes, nil
	}
	if len(input)%192 != 0 {
		return nil, fmt.Errorf("bad size")
	}

	var buf []byte

	num := len(input) / 192
	ar := make([]*bn256.G1, num)
	br := make([]*bn256.G2, num)

	for i := 0; i < num; i++ {
		a0 := new(bn256.G1)
		b0 := new(bn256.G2)

		buf, input = b.get(input, 64)
		if _, err := a0.Unmarshal(buf); err != nil {
			return nil, err
		}

		buf, input = b.get(input, 128)
		if _, err := b0.Unmarshal(buf); err != nil {
			return nil, err
		}

		ar[i] = a0
		br[i] = b0
	}

	if bn256.PairingCheck(ar, br) {
		return trueBytes, nil
	}
	return falseBytes, nil
}
