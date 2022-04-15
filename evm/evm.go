package evm

import (
	"math/big"

	"github.com/ethereum/evmc/v10/bindings/go/evmc"
)

type EVM struct {
	Host evmc.HostContext
	Rev  evmc.Revision
}

// Run implements the runtime interface
func (e *EVM) Run(typ evmc.CallKind, recipient evmc.Address, sender evmc.Address, value *big.Int, input []byte, gas int64, depth int, static bool, codeAddress evmc.Address) ([]byte, int64, error) {

	s := acquireState()
	s.resetReturnData()

	//contract.msg = c
	s.Address = recipient
	s.Caller = sender
	s.Depth = depth
	s.Value = value
	s.Static = static

	if typ == evmc.Create || typ == evmc.Create2 {
		s.Input = nil
	} else {
		s.Input = input
	}

	if typ == evmc.Create || typ == evmc.Create2 {
		// code creation
		s.code = input
	} else {
		// code call
		s.code = e.Host.GetCode(codeAddress)
	}

	s.gas = uint64(gas)
	s.host = e.Host
	s.rev = e.Rev
	s.bitmap.setCode(s.code)

	ret, err := s.Run()

	// We are probably doing this append magic to make sure that the slice doesn't have more capacity than it needs
	var returnValue []byte
	returnValue = append(returnValue[:0], ret...)

	gasLeft := s.gas

	releaseState(s)

	if err != nil && err != ErrExecutionReverted {
		gasLeft = 0
	}

	return returnValue, int64(gasLeft), err
}
