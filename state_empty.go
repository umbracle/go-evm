package state

import "github.com/ethereum/evmc/v10/bindings/go/evmc"

type EmptyState struct {
}

func (e *EmptyState) GetStorage(addr evmc.Address, root evmc.Hash, key evmc.Hash) evmc.Hash {
	return evmc.Hash{}
}

func (e *EmptyState) GetAccount(addr evmc.Address) (*Account, error) {
	return nil, nil
}
