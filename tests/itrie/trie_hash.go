package itrie

import (
	"fmt"

	"github.com/umbracle/ethgo"
	"github.com/umbracle/fastrlp"
	state "github.com/umbracle/go-evm"
)

func (t *Txn) Hash() ([]byte, error) {
	if t.root == nil {
		return state.EmptyRootHash[:], nil
	}

	var root []byte

	arena := &fastrlp.Arena{}
	val := t.hash(t.root, arena, 0)

	if val.Type() == fastrlp.TypeBytes {
		if val.Len() != 32 {
			root = ethgo.Keccak256(val.Raw())
		} else {
			root = make([]byte, 32)
			copy(root, val.Raw())
		}
	} else {
		tmp := val.MarshalTo(nil)
		root = ethgo.Keccak256(tmp)
	}
	return root, nil
}

func (t *Txn) hash(node Node, a *fastrlp.Arena, d int) *fastrlp.Value {
	var val *fastrlp.Value

	switch n := node.(type) {
	case *ValueNode:
		return a.NewCopyBytes(n.buf)

	case *ShortNode:
		child := t.hash(n.child, a, d+1)

		val = a.NewArray()
		val.Set(a.NewBytes(hexToCompact(n.key)))
		val.Set(child)

	case *FullNode:
		val = a.NewArray()

		for _, i := range n.children {
			if i == nil {
				val.Set(a.NewNull())
			} else {
				val.Set(t.hash(i, a, d+1))
			}
		}

		// Add the value
		if n.value == nil {
			val.Set(a.NewNull())
		} else {
			val.Set(t.hash(n.value, a, d+1))
		}

	default:
		panic(fmt.Sprintf("unknown node type %v", n))
	}

	if val.Len() < 32 {
		return val
	}

	// marshal RLP value
	buf := val.MarshalTo(nil)
	tmp := ethgo.Keccak256(buf)
	return a.NewCopyBytes(tmp)
}
