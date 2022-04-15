package state

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"strings"

	"github.com/ethereum/evmc/v10/bindings/go/evmc"
	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/umbracle/ethgo"
)

type Snapshot interface {
	GetStorage(addr evmc.Address, root evmc.Hash, key evmc.Hash) evmc.Hash
	GetAccount(addr evmc.Address) (*Account, error)
}

// StateObject is the internal representation of the account
type stateObject struct {
	Account   *Account
	Code      []byte
	Suicide   bool
	Deleted   bool
	DirtyCode bool
	Txn       *iradix.Txn
}

func (s *stateObject) Empty() bool {
	return s.Account.Nonce == 0 && s.Account.Balance.Sign() == 0 && bytes.Equal(s.Account.CodeHash, EmptyCodeHash[:])
}

// Copy makes a copy of the state object
func (s *stateObject) Copy() *stateObject {
	ss := new(stateObject)

	// copy account
	ss.Account = s.Account.Copy()

	ss.Suicide = s.Suicide
	ss.Deleted = s.Deleted
	ss.DirtyCode = s.DirtyCode
	ss.Code = s.Code

	if s.Txn != nil {
		ss.Txn = s.Txn.CommitOnly().Txn()
	}
	return ss
}

// Object is the serialization of the radix object (can be merged to StateObject?).
type Object struct {
	Address   evmc.Address
	CodeHash  evmc.Hash
	Balance   *big.Int
	Root      evmc.Hash
	Nonce     uint64
	Deleted   bool
	DirtyCode bool
	Code      []byte
	Storage   []*StorageObject
}

// StorageObject is an entry in the storage
type StorageObject struct {
	Deleted bool
	Key     []byte
	Val     []byte
}

type Output struct {
	Logs            []*Log
	Success         bool
	GasLeft         uint64
	ContractAddress evmc.Address
	ReturnValue     []byte
}

type Log struct {
	Address evmc.Address
	Topics  []evmc.Hash
	Data    []byte
}

// Account is an object we can retrieve from the state
type Account struct {
	Nonce    uint64
	Balance  *big.Int
	Root     evmc.Hash
	CodeHash []byte
	Code     []byte
}

func (a *Account) Copy() *Account {
	aa := new(Account)

	aa.Balance = big.NewInt(1).SetBytes(a.Balance.Bytes())
	aa.Nonce = a.Nonce
	aa.CodeHash = a.CodeHash
	aa.Root = a.Root

	return aa
}

const HashLength = 32

func bytesToHash(b []byte) evmc.Hash {
	var h evmc.Hash

	size := len(b)
	min := min(size, HashLength)

	copy(h[HashLength-min:], b[len(b)-min:])
	return h
}

func StringToHash(str string) evmc.Hash {
	return bytesToHash(stringToBytes(str))
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

func stringToBytes(str string) []byte {
	str = strings.TrimPrefix(str, "0x")
	if len(str)%2 == 1 {
		str = "0" + str
	}
	b, _ := hex.DecodeString(str)
	return b
}

var (
	EmptyCodeHash = bytesToHash(ethgo.Keccak256(nil))

	// EmptyRootHash is the root when there are no transactions
	EmptyRootHash = StringToHash("0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
)

type Message struct {
	Nonce    uint64
	GasPrice *big.Int
	Gas      uint64
	To       *evmc.Address
	Value    *big.Int
	Input    []byte
	From     evmc.Address
}

func (t *Message) IsContractCreation() bool {
	return t.To == nil
}

// Contract is the instance being called
type Contract struct {
	Type        evmc.CallKind
	CodeAddress evmc.Address
	Address     evmc.Address
	Caller      evmc.Address
	Depth       int
	Value       *big.Int
	Input       []byte
	Gas         uint64
	Static      bool
	Salt        evmc.Hash
}

func NewContract(typ evmc.CallKind, depth int, from evmc.Address, to evmc.Address, value *big.Int, gas uint64, input []byte) *Contract {
	f := &Contract{
		Type:        typ,
		Caller:      from,
		CodeAddress: to,
		Address:     to,
		Gas:         gas,
		Value:       value,
		Input:       input,
		Depth:       depth,
	}
	return f
}

func NewContractCreation(depth int, from evmc.Address, to evmc.Address, value *big.Int, gas uint64, code []byte) *Contract {
	c := NewContract(evmc.Create, depth, from, to, value, gas, code)
	return c
}

func NewContractCall(depth int, from evmc.Address, to evmc.Address, value *big.Int, gas uint64, input []byte) *Contract {
	c := NewContract(evmc.Call, depth, from, to, value, gas, input)
	return c
}
