package state

import (
	"fmt"
	"math/big"

	"github.com/ethereum/evmc/v10/bindings/go/evmc"
	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/umbracle/ethgo"
)

var (
	// logIndex is the index of the logs in the trie
	logIndex = bytesToHash([]byte{2})

	// refundIndex is the index of the refund
	refundIndex = bytesToHash([]byte{3})
)

// Txn is a reference of the state
type Txn struct {
	snapshot  Snapshot
	snapshots []*iradix.Tree
	txn       *iradix.Txn
	rev       evmc.Revision
}

func NewTxn(snapshot Snapshot) *Txn {
	return newTxn(snapshot)
}

func newTxn(snapshot Snapshot) *Txn {
	i := iradix.New()

	return &Txn{
		snapshot:  snapshot,
		snapshots: []*iradix.Tree{},
		txn:       i.Txn(),
	}
}

// Snapshot takes a snapshot at this point in time
func (txn *Txn) Snapshot() int {
	t := txn.txn.CommitOnly()

	id := len(txn.snapshots)
	txn.snapshots = append(txn.snapshots, t)

	return id
}

// RevertToSnapshot reverts to a given snapshot
func (txn *Txn) RevertToSnapshot(id int) {
	if id > len(txn.snapshots) {
		panic("")
	}

	tree := txn.snapshots[id]
	txn.txn = tree.Txn()
}

// GetAccount returns an account
func (txn *Txn) GetAccount(addr evmc.Address) (*Account, bool) {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return nil, false
	}
	return object.Account, true
}

func (txn *Txn) getStateObject(addr evmc.Address) (*stateObject, bool) {
	// Try to get state from radix tree which holds transient states during block processing first
	val, exists := txn.txn.Get(addr[:])
	if exists {
		obj := val.(*stateObject)
		if obj.Deleted {
			return nil, false
		}
		return obj.Copy(), true
	}

	account, err := txn.snapshot.GetAccount(addr)
	if err != nil {
		return nil, false
	}
	if account == nil {
		return nil, false
	}
	obj := &stateObject{
		Account: account.Copy(),
		Code:    account.Code,
	}
	return obj, true
}

func (txn *Txn) upsertAccount(addr evmc.Address, create bool, f func(object *stateObject)) {
	object, exists := txn.getStateObject(addr)
	if !exists && create {
		object = &stateObject{
			Account: &Account{
				Balance:  big.NewInt(0),
				CodeHash: EmptyCodeHash[:],
				Root:     EmptyRootHash,
			},
		}
	}

	// run the callback to modify the account
	f(object)

	if object != nil {
		txn.txn.Insert(addr[:], object)
	}
}

func (txn *Txn) AddSealingReward(addr evmc.Address, balance *big.Int) {
	txn.upsertAccount(addr, true, func(object *stateObject) {
		if object.Suicide {
			*object = *newStateObject(txn)
			object.Account.Balance.SetBytes(balance.Bytes())
		} else {
			object.Account.Balance.Add(object.Account.Balance, balance)
		}
	})
}

// AddBalance adds balance
func (txn *Txn) AddBalance(addr evmc.Address, balance *big.Int) {
	txn.upsertAccount(addr, true, func(object *stateObject) {
		object.Account.Balance.Add(object.Account.Balance, balance)
	})
}

var errNotEnoughFunds = fmt.Errorf("not enough funds for transfer with given value")

// SubBalance reduces the balance at address addr by amount
func (txn *Txn) SubBalance(addr evmc.Address, amount *big.Int) error {
	// If we try to reduce balance by 0, then it's a noop
	if amount.Sign() == 0 {
		return nil
	}

	// Check if we have enough balance to deduce amount from
	if balance := txn.GetBalance(addr); balance.Cmp(amount) < 0 {
		return errNotEnoughFunds
	}

	txn.upsertAccount(addr, true, func(object *stateObject) {
		object.Account.Balance.Sub(object.Account.Balance, amount)
	})

	return nil
}

// SetBalance sets the balance
func (txn *Txn) SetBalance(addr evmc.Address, balance *big.Int) {
	//fmt.Printf("SET BALANCE: %s %s\n", addr.String(), balance.String())
	txn.upsertAccount(addr, true, func(object *stateObject) {
		object.Account.Balance.SetBytes(balance.Bytes())
	})
}

// GetBalance returns the balance of an address
func (txn *Txn) GetBalance(addr evmc.Address) *big.Int {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return big.NewInt(0)
	}
	return object.Account.Balance
}

func (txn *Txn) EmitLog(addr evmc.Address, topics []evmc.Hash, data []byte) {
	log := &Log{
		Address: addr,
		Topics:  topics,
		Data:    append([]byte{}, data...),
	}

	var logs []*Log
	val, exists := txn.txn.Get(logIndex[:])
	if !exists {
		logs = []*Log{}
	} else {
		logs = val.([]*Log)
	}

	logs = append(logs, log)
	txn.txn.Insert(logIndex[:], logs)
}

// State

var zeroHash evmc.Hash

func (txn *Txn) isRevision(rev evmc.Revision) bool {
	return rev <= txn.rev
}

func (txn *Txn) SetStorage(addr evmc.Address, key evmc.Hash, value evmc.Hash) (status evmc.StorageStatus) {
	oldValue := txn.GetState(addr, key)
	if oldValue == value {
		return evmc.StorageUnchanged
	}

	current := oldValue                          // current - storage dirtied by previous lines of this contract
	original := txn.GetCommittedState(addr, key) // storage slot before this transaction started

	txn.SetState(addr, key, value)

	isIstanbul := txn.isRevision(evmc.Istanbul)
	legacyGasMetering := !isIstanbul && (txn.isRevision(evmc.Petersburg) || !txn.isRevision(evmc.Constantinople))

	if legacyGasMetering {
		status = evmc.StorageModified
		if oldValue == zeroHash {
			return evmc.StorageAdded
		} else if value == zeroHash {
			txn.AddRefund(15000)
			return evmc.StorageDeleted
		}
		return evmc.StorageModified
	}

	if original == current {
		if original == zeroHash { // create slot (2.1.1)
			return evmc.StorageAdded
		}
		if value == zeroHash { // delete slot (2.1.2b)
			txn.AddRefund(15000)
			return evmc.StorageDeleted
		}
		return evmc.StorageModified
	}
	if original != zeroHash { // Storage slot was populated before this transaction started
		if current == zeroHash { // recreate slot (2.2.1.1)
			txn.SubRefund(15000)
		} else if value == zeroHash { // delete slot (2.2.1.2)
			txn.AddRefund(15000)
		}
	}
	if original == value {
		if original == zeroHash { // reset to original nonexistent slot (2.2.2.1)
			// Storage was used as memory (allocation and deallocation occurred within the same contract)
			if isIstanbul {
				txn.AddRefund(19200)
			} else {
				txn.AddRefund(19800)
			}
		} else { // reset to original existing slot (2.2.2.2)
			if isIstanbul {
				txn.AddRefund(4200)
			} else {
				txn.AddRefund(4800)
			}
		}
	}
	return evmc.StorageModifiedAgain
}

// SetState change the state of an address
func (txn *Txn) SetState(addr evmc.Address, key, value evmc.Hash) {
	txn.upsertAccount(addr, true, func(object *stateObject) {
		if object.Txn == nil {
			object.Txn = iradix.New().Txn()
		}

		if value == zeroHash {
			object.Txn.Insert(key[:], nil)
		} else {
			object.Txn.Insert(key[:], value[:])
		}
	})
}

// GetState returns the state of the address at a given key
func (txn *Txn) GetState(addr evmc.Address, key evmc.Hash) evmc.Hash {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return evmc.Hash{}
	}

	// Try to get account state from radix tree first
	// Because the latest account state should be in in-memory radix tree
	// if account state update happened in previous transactions of same block
	if object.Txn != nil {
		if val, ok := object.Txn.Get(key[:]); ok {
			if val == nil {
				return evmc.Hash{}
			}
			return bytesToHash(val.([]byte))
		}
	}
	return txn.snapshot.GetStorage(addr, object.Account.Root, key)
}

// Nonce

// IncrNonce increases the nonce of the address
func (txn *Txn) IncrNonce(addr evmc.Address) {
	txn.upsertAccount(addr, true, func(object *stateObject) {
		object.Account.Nonce++
	})
}

// SetNonce reduces the balance
func (txn *Txn) SetNonce(addr evmc.Address, nonce uint64) {
	txn.upsertAccount(addr, true, func(object *stateObject) {
		object.Account.Nonce = nonce
	})
}

// GetNonce returns the nonce of an addr
func (txn *Txn) GetNonce(addr evmc.Address) uint64 {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return 0
	}
	return object.Account.Nonce
}

// Code

// SetCode sets the code for an address
func (txn *Txn) SetCode(addr evmc.Address, code []byte) {
	txn.upsertAccount(addr, true, func(object *stateObject) {
		object.Account.CodeHash = ethgo.Keccak256(code)
		object.DirtyCode = true
		object.Code = code
	})
}

func (txn *Txn) GetCode(addr evmc.Address) []byte {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return nil
	}
	if object.DirtyCode {
		return object.Code
	}
	return object.Code
}

func (txn *Txn) GetCodeSize(addr evmc.Address) int {
	return len(txn.GetCode(addr))
}

func (txn *Txn) GetCodeHash(addr evmc.Address) (res evmc.Hash) {
	if txn.empty(addr) {
		return
	}
	object, exists := txn.getStateObject(addr)
	if !exists {
		return
	}
	copy(res[:], object.Account.CodeHash)
	return
}

// Suicide marks the given account as suicided
func (txn *Txn) Suicide(addr evmc.Address) bool {
	var suicided bool
	txn.upsertAccount(addr, false, func(object *stateObject) {
		if object == nil || object.Suicide {
			suicided = false
		} else {
			suicided = true
			object.Suicide = true
		}
		if object != nil {
			object.Account.Balance = new(big.Int)
		}
	})
	return suicided
}

// HasSuicided returns true if the account suicided
func (txn *Txn) HasSuicided(addr evmc.Address) bool {
	object, exists := txn.getStateObject(addr)
	return exists && object.Suicide
}

// Refund
func (txn *Txn) AddRefund(gas uint64) {
	refund := txn.GetRefund() + gas
	txn.txn.Insert(refundIndex[:], refund)
}

func (txn *Txn) SubRefund(gas uint64) {
	refund := txn.GetRefund() - gas
	txn.txn.Insert(refundIndex[:], refund)
}

func (txn *Txn) Logs() []*Log {
	data, exists := txn.txn.Get(logIndex[:])
	if !exists {
		return nil
	}
	txn.txn.Delete(logIndex[:])
	return data.([]*Log)
}

func (txn *Txn) GetRefund() uint64 {
	data, exists := txn.txn.Get(refundIndex[:])
	if !exists {
		return 0
	}
	return data.(uint64)
}

// GetCommittedState returns the state of the address in the trie
func (txn *Txn) GetCommittedState(addr evmc.Address, key evmc.Hash) evmc.Hash {
	obj, ok := txn.getStateObject(addr)
	if !ok {
		return evmc.Hash{}
	}
	return txn.snapshot.GetStorage(addr, obj.Account.Root, key)
}

func (txn *Txn) TouchAccount(addr evmc.Address) {
	txn.upsertAccount(addr, true, func(obj *stateObject) {

	})
}

func (txn *Txn) AccountExists(addr evmc.Address) bool {
	if txn.rev >= evmc.SpuriousDragon {
		return !txn.empty(addr)
	}
	_, exists := txn.getStateObject(addr)
	return exists
}

func (txn *Txn) empty(addr evmc.Address) bool {
	obj, exists := txn.getStateObject(addr)
	if !exists {
		return true
	}
	return obj.Empty()
}

func newStateObject(txn *Txn) *stateObject {
	return &stateObject{
		Account: &Account{
			Balance:  big.NewInt(0),
			CodeHash: EmptyCodeHash[:],
			Root:     EmptyRootHash,
		},
	}
}

func (txn *Txn) CreateAccount(addr evmc.Address) {
	obj := &stateObject{
		Account: &Account{
			Balance:  big.NewInt(0),
			CodeHash: EmptyCodeHash[:],
			Root:     EmptyRootHash,
		},
	}

	prev, ok := txn.getStateObject(addr)
	if ok {
		obj.Account.Balance.SetBytes(prev.Account.Balance.Bytes())
	}

	txn.txn.Insert(addr[:], obj)
}

func (txn *Txn) CleanDeleteObjects(deleteEmptyObjects bool) {
	remove := [][]byte{}
	txn.txn.Root().Walk(func(k []byte, v interface{}) bool {
		a, ok := v.(*stateObject)
		if !ok {
			return false
		}
		if a.Suicide || a.Empty() && deleteEmptyObjects {
			remove = append(remove, k)
		}
		return false
	})

	for _, k := range remove {
		v, ok := txn.txn.Get(k)
		if !ok {
			panic("it should not happen")
		}
		obj, ok := v.(*stateObject)
		if !ok {
			panic("it should not happen")
		}

		obj2 := obj.Copy()
		obj2.Deleted = true
		txn.txn.Insert(k, obj2)
	}

	// delete refunds
	txn.txn.Delete(refundIndex[:])
}

func (txn *Txn) Commit() []*Object {
	x := txn.txn.Commit()

	// Do a more complex thing for now
	objs := []*Object{}
	x.Root().Walk(func(k []byte, v interface{}) bool {
		a, ok := v.(*stateObject)
		if !ok {
			// We also have logs, avoid those
			return false
		}

		// k is an array of 20 bytes
		var addr evmc.Address
		copy(addr[:], k)

		var codeHash evmc.Hash
		copy(codeHash[:], a.Account.CodeHash)

		obj := &Object{
			Nonce:     a.Account.Nonce,
			Address:   addr,
			Balance:   a.Account.Balance,
			Root:      a.Account.Root,
			CodeHash:  codeHash,
			DirtyCode: a.DirtyCode,
			Code:      a.Code,
		}
		if a.Deleted {
			obj.Deleted = true
		} else {
			if a.Txn != nil {
				a.Txn.Root().Walk(func(k []byte, v interface{}) bool {
					store := &StorageObject{Key: k}
					if v == nil {
						store.Deleted = true
					} else {
						store.Val = v.([]byte)
					}
					obj.Storage = append(obj.Storage, store)
					return false
				})
			}
		}

		objs = append(objs, obj)
		return false
	})

	return objs
}
