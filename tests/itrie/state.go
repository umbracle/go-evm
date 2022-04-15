package itrie

import (
	"bytes"
	"math/big"

	"github.com/ethereum/evmc/v10/bindings/go/evmc"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/fastrlp"
	state "github.com/umbracle/go-evm"
)

// Account is an object we can retrieve from the state
type Account struct {
	Nonce    uint64
	Balance  *big.Int
	Root     evmc.Hash
	CodeHash []byte
}

func (a *Account) MarshalWith(ar *fastrlp.Arena) *fastrlp.Value {
	v := ar.NewArray()
	v.Set(ar.NewUint(a.Nonce))
	v.Set(ar.NewBigInt(a.Balance))
	v.Set(ar.NewBytes(a.Root[:]))
	v.Set(ar.NewBytes(a.CodeHash))
	return v
}

func commitStorage(data []*state.StorageObject) (res evmc.Hash) {
	arena := &fastrlp.Arena{}
	localTxn := NewTxn()

	for _, entry := range data {
		k := ethgo.Keccak256(entry.Key)
		if entry.Deleted {
			localTxn.Delete(k)
		} else {
			vv := arena.NewBytes(bytes.TrimLeft(entry.Val, "\x00"))
			localTxn.Insert(k, vv.MarshalTo(nil))
		}
	}

	root, _ := localTxn.Hash()
	copy(res[:], root)
	return
}

func Commit(objs []*state.Object) []byte {
	tt := NewTxn()

	arena := &fastrlp.Arena{}
	for _, obj := range objs {
		addrHash := ethgo.Keccak256(obj.Address[:])

		if obj.Deleted {
			tt.Delete(addrHash)
		} else {
			account := Account{
				Balance:  obj.Balance,
				Nonce:    obj.Nonce,
				CodeHash: obj.CodeHash[:],
			}

			if len(obj.Storage) != 0 {
				account.Root = commitStorage(obj.Storage)
			} else {
				account.Root = state.EmptyRootHash
			}

			data := account.MarshalWith(arena).MarshalTo(nil)
			tt.Insert(addrHash, data)
		}
	}

	root, _ := tt.Hash()
	return root
}
