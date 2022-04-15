package tests

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/ethereum/evmc/v10/bindings/go/evmc"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/ethgo"
	state "github.com/umbracle/go-evm"
	itrie "github.com/umbracle/go-evm/tests/itrie"
)

var (
	stateTests       = "GeneralStateTests"
	legacyStateTests = "LegacyTests/Constantinople/GeneralStateTests"
)

type stateCase struct {
	Env         *env                        `json:"env"`
	Pre         map[argAddr]*GenesisAccount `json:"pre"`
	Post        map[string]postState        `json:"post"`
	Transaction *stTransaction              `json:"transaction"`
}

type wrapper struct {
	cc map[argAddr]*GenesisAccount
}

func newWrapper(cc map[argAddr]*GenesisAccount) *wrapper {
	w := &wrapper{
		cc: cc,
	}
	return w
}

func (w *wrapper) GetStorage(addr evmc.Address, root evmc.Hash, key evmc.Hash) evmc.Hash {
	if root == state.EmptyRootHash {
		return evmc.Hash{}
	}
	acct, ok := w.cc[argAddr(addr)]
	if !ok {
		return evmc.Hash{}
	}
	val, ok := acct.Storage[argHash(key)]
	if !ok {
		return evmc.Hash{}
	}
	return evmc.Hash(val)
}

func (w *wrapper) GetAccount(addr evmc.Address) (*state.Account, error) {
	acct, ok := w.cc[argAddr(addr)]
	if !ok {
		return nil, nil
	}
	if acct == nil {
		return nil, nil
	}
	newAcct := &state.Account{
		Balance:  acct.Balance.Big(),
		Nonce:    acct.Nonce.Uint64(),
		CodeHash: ethgo.Keccak256(acct.Code),
		Root:     evmc.Hash{},
		Code:     acct.Code,
	}
	return newAcct, nil
}

func RunSpecificTest(file string, t *testing.T, c stateCase, name, fork string, index int, p postEntry) {
	// fmt.Println(file, name, fork, index)

	env := c.Env.ToEnv(t)

	// find the fork
	goahead, ok := Forks2[fork]
	if !ok {
		t.Fatalf("config %s not found", fork)
	}
	rev := goahead(int(env.Number))

	msg, err := c.Transaction.At(p.Indexes)
	assert.NoError(t, err)

	runtimeCtx := env
	runtimeCtx.ChainID = 1

	wr := newWrapper(c.Pre)

	opts := []state.ConfigOption{
		state.WithRevision(rev),
		state.WithContext(runtimeCtx),
		state.WithState(wr),
	}
	transition := state.NewTransition(opts...)

	result, err := transition.Write(msg)
	assert.NoError(t, err)

	objs := transition.Commit()
	root := computeRoot(c.Pre, objs)

	if !bytes.Equal(root, p.Root[:]) {
		t.Fatalf("root mismatch (%s %s %s %d): expected %s but found %s", file, name, fork, index, p.Root, hex.EncodeToString(root))
	}
	if logs := rlpHashLogs(result.Logs); !bytes.Equal(logs[:], p.Logs[:]) {
		t.Fatalf("logs mismatch (%s, %s %d): expected %s but found %s", name, fork, index, p.Logs, logs[:])
	}
}

var zeroHash = argHash{}

func computeRoot(pre map[argAddr]*GenesisAccount, post []*state.Object) []byte {

	resMap := map[evmc.Address]*state.Object{}

	// add pre data
	for addr, data := range pre {
		obj := &state.Object{
			Address:  evmc.Address(addr),
			Nonce:    data.Nonce.Uint64(),
			Balance:  data.Balance.Big(),
			Root:     state.EmptyRootHash,
			CodeHash: state.EmptyCodeHash,
			Storage:  []*state.StorageObject{},
		}
		if len(data.Code) != 0 {
			obj.Code = data.Code
			copy(obj.CodeHash[:], ethgo.Keccak256(data.Code))
		}
		for k, v := range data.Storage {
			key := append([]byte{}, k[:]...)
			val := append([]byte{}, v[:]...)

			entry := &state.StorageObject{
				Key: key,
			}
			if v == zeroHash {
				entry.Deleted = true
			} else {
				entry.Val = val
			}
			obj.Storage = append(obj.Storage, entry)
		}
		resMap[evmc.Address(addr)] = obj

	}

	// merge post data
	for _, raw := range post {
		var obj *state.Object
		var ok bool

		if obj, ok = resMap[raw.Address]; !ok {
			obj = &state.Object{
				Address: raw.Address,
				Root:    state.EmptyRootHash,
				Storage: []*state.StorageObject{},

				// in this case the object already set the hash
				Code:     raw.Code,
				CodeHash: raw.CodeHash,
			}
			resMap[raw.Address] = obj
		}

		obj.Nonce = raw.Nonce
		obj.Balance = raw.Balance
		obj.Deleted = raw.Deleted
		obj.CodeHash = raw.CodeHash
		obj.DirtyCode = raw.DirtyCode

		if obj.DirtyCode {
			// if the storage is set, all the state is gone
			obj.Storage = []*state.StorageObject{}
		}

		// we just override the values since trie will handle it
		obj.Storage = append(obj.Storage, raw.Storage...)
	}

	// convert to array
	objs := []*state.Object{}
	for _, obj := range resMap {
		objs = append(objs, obj)
	}

	root := itrie.Commit(objs)
	return root
}

func TestState(t *testing.T) {
	long := []string{
		"static_Call50000",
		"static_Return50000",
		"static_Call1MB",
		"stQuadraticComplexityTest",
		"stTimeConsuming",
	}

	skip := []string{
		"RevertPrecompiledTouch",
		"failed_tx_xcf416c53",
	}

	// There are two folders in spec tests, one for the current tests for the Istanbul fork
	// and one for the legacy tests for the other forks
	folders, err := listFolders(stateTests, legacyStateTests)
	if err != nil {
		t.Fatal(err)
	}

	for _, folder := range folders {
		t.Run(folder, func(t *testing.T) {
			files, err := listFiles(folder)
			if err != nil {
				t.Fatal(err)
			}

			for _, file := range files {
				if !strings.HasSuffix(file, ".json") {
					continue
				}

				if contains(long, file) && testing.Short() {
					t.Log("Long tests are skipped in short mode")
					continue
				}

				if contains(skip, file) {
					t.Log("Skip test")
					continue
				}

				data, err := ioutil.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}

				var c map[string]stateCase
				if err := json.Unmarshal(data, &c); err != nil {
					t.Fatal(err)
				}

				for name, i := range c {
					for fork, f := range i.Post {
						for indx, e := range f {
							RunSpecificTest(file, t, i, name, fork, indx, e)
						}
					}
				}
			}
		})
	}
}

func rlpHashLogs(logs []*state.Log) []byte {
	return ethgo.Keccak256(MarshalLogsWith(logs))
}
