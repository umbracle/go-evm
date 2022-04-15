package state

import (
	"math/big"

	"github.com/ethereum/evmc/v10/bindings/go/evmc"
	"github.com/umbracle/ethgo"
)

type Config struct {
	GetHash    GetHashByNumber
	Ctx        TxContext
	Rev        evmc.Revision
	State      Snapshot
	Cheatcodes []Cheatcode
}

func DefaultConfig() *Config {
	c := &Config{
		GetHash:    getHashDefault,
		Ctx:        TxContext{},
		Rev:        evmc.Istanbul,
		State:      &EmptyState{},
		Cheatcodes: []Cheatcode{},
	}
	return c
}

type ConfigOption func(*Config)

func WithGetHash(hash GetHashByNumber) ConfigOption {
	return func(c *Config) {
		c.GetHash = hash
	}
}

func WithContext(ctx TxContext) ConfigOption {
	return func(c *Config) {
		c.Ctx = ctx
	}
}

func WithRevision(rev evmc.Revision) ConfigOption {
	return func(c *Config) {
		c.Rev = rev
	}
}

func WithState(state Snapshot) ConfigOption {
	return func(c *Config) {
		c.State = state
	}
}

type Cheatcode interface {
	CanRun(addr evmc.Address) bool
	Run(addr evmc.Address, input []byte)
}

func WithCheatcode(cheat Cheatcode) ConfigOption {
	return func(c *Config) {
		c.Cheatcodes = append(c.Cheatcodes, cheat)
	}
}

func getHashDefault(n uint64) (res evmc.Hash) {
	hash := ethgo.Keccak256([]byte(big.NewInt(int64(n)).String()))
	copy(res[:], hash)
	return
}
