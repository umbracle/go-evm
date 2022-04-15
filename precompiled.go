package state

import (
	"errors"

	"github.com/ethereum/evmc/v10/bindings/go/evmc"
	"github.com/umbracle/go-evm/precompiled"
)

var (
	addr1 = evmc.Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	addr2 = evmc.Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
	addr3 = evmc.Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3}
	addr4 = evmc.Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4}
	addr5 = evmc.Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 5}
	addr6 = evmc.Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 6}
	addr7 = evmc.Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7}
	addr8 = evmc.Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8}
	addr9 = evmc.Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 9}
)

var precompiledContracts map[evmc.Address]contract

func register(addr evmc.Address, b contract) {
	if len(precompiledContracts) == 0 {
		precompiledContracts = map[evmc.Address]contract{}
	}
	precompiledContracts[addr] = b
}

func init() {
	register(addr1, &precompiled.Ecrecover{})
	register(addr2, &precompiled.Sha256h{})
	register(addr3, &precompiled.Ripemd160h{})
	register(addr4, &precompiled.Identity{})

	// Byzantium fork
	register(addr5, &precompiled.ModExp{})
	register(addr6, &precompiled.Bn256Add{})
	register(addr7, &precompiled.Bn256Mul{})
	register(addr8, &precompiled.Bn256Pairing{})

	// Istanbul fork
	register(addr9, &precompiled.Blake2f{})
}

type contract interface {
	Gas(input []byte, rev evmc.Revision) uint64
	Run(input []byte) ([]byte, error)
}

// runPrecompiled runs an execution
func runPrecompiled(codeAddress evmc.Address, input []byte, gas uint64, rev evmc.Revision) ([]byte, int64, error) {
	contract := precompiledContracts[codeAddress]
	gasCost := contract.Gas(input, rev)

	// In the case of not enough gas for precompiled execution we return ErrOutOfGas
	if gas < gasCost {
		return nil, 0, errors.New("out of gas")
	}

	gas = gas - gasCost
	returnValue, err := contract.Run(input)
	if err != nil {
		return nil, 0, err
	}
	return returnValue, int64(gas), nil
}
