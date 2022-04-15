package itrie

import (
	"fmt"
)

// Node represents a node reference
type Node interface {
	IsNode()
}

// ValueNode is a leaf on the merkle-trie
type ValueNode struct {
	buf []byte
}

func (v *ValueNode) IsNode() {
}

// ShortNode is an extension or short node
type ShortNode struct {
	key   []byte
	child Node
}

func (s *ShortNode) IsNode() {
}

// FullNode is a node with several children
type FullNode struct {
	value    Node
	children [16]Node
}

func (f *FullNode) IsNode() {
}

func (f *FullNode) copy() *FullNode {
	nc := &FullNode{}
	nc.value = f.value
	copy(nc.children[:], f.children[:])
	return nc
}

func (f *FullNode) replaceEdge(idx byte, e Node) {
	if idx == 16 {
		f.value = e
	} else {
		f.children[idx] = e
	}
}

func (f *FullNode) setEdge(idx byte, e Node) {
	if idx == 16 {
		f.value = e
	} else {
		f.children[idx] = e
	}
}

func (f *FullNode) getEdge(idx byte) Node {
	if idx == 16 {
		return f.value
	} else {
		return f.children[idx]
	}
}

func NewTxn() *Txn {
	return &Txn{}
}

type Txn struct {
	root Node
}

func (t *Txn) Insert(key, value []byte) {
	root := t.insert(t.root, bytesToPath(key), value)
	if root != nil {
		t.root = root
	}
}

func (t *Txn) insert(node Node, search, value []byte) Node {
	switch n := node.(type) {
	case nil:
		// NOTE, this only happens with the full node
		if len(search) == 0 {
			v := &ValueNode{}
			v.buf = make([]byte, len(value))
			copy(v.buf, value)
			return v
		} else {
			return &ShortNode{
				key:   search,
				child: t.insert(nil, nil, value),
			}
		}

	case *ValueNode:
		if len(search) == 0 {
			v := &ValueNode{}
			v.buf = make([]byte, len(value))
			copy(v.buf, value)
			return v
		} else {
			b := t.insert(&FullNode{value: n}, search, value)
			return b
		}

	case *ShortNode:
		plen := prefixLen(search, n.key)
		if plen == len(n.key) {
			// Keep this node as is and insert to child
			child := t.insert(n.child, search[plen:], value)
			return &ShortNode{key: n.key, child: child}

		} else {
			// Introduce a new branch
			b := FullNode{}
			if len(n.key) > plen+1 {
				b.setEdge(n.key[plen], &ShortNode{key: n.key[plen+1:], child: n.child})
			} else {
				b.setEdge(n.key[plen], n.child)
			}

			child := t.insert(&b, search[plen:], value)

			if plen == 0 {
				return child
			} else {
				return &ShortNode{key: search[:plen], child: child}
			}
		}

	case *FullNode:
		// override node since we only do one round
		b := n

		if len(search) == 0 {
			b.value = t.insert(b.value, nil, value)
			return b
		} else {
			k := search[0]
			child := n.getEdge(k)
			newChild := t.insert(child, search[1:], value)
			if child == nil {
				b.setEdge(k, newChild)
			} else {
				b.replaceEdge(k, newChild)
			}
			return b
		}

	default:
		panic(fmt.Sprintf("unknown node type %v", n))
	}
}

func (t *Txn) Delete(key []byte) {
	root, ok := t.delete(t.root, bytesToPath(key))
	if ok {
		t.root = root
	}
}

func (t *Txn) delete(node Node, search []byte) (Node, bool) {
	switch n := node.(type) {
	case nil:
		return nil, false

	case *ShortNode:
		// n.hash = n.hash[:0]

		plen := prefixLen(search, n.key)
		if plen == len(search) {
			return nil, true
		}
		if plen == 0 {
			return nil, false
		}

		child, ok := t.delete(n.child, search[plen:])
		if !ok {
			return nil, false
		}
		if child == nil {
			return nil, true
		}
		if short, ok := child.(*ShortNode); ok {
			// merge nodes
			return &ShortNode{key: concat(n.key, short.key), child: short.child}, true
		} else {
			// full node
			return &ShortNode{key: n.key, child: child}, true
		}

	case *ValueNode:
		if len(search) != 0 {
			return nil, false
		}
		return nil, true

	case *FullNode:
		n = n.copy()

		key := search[0]
		newChild, ok := t.delete(n.getEdge(key), search[1:])
		if !ok {
			return nil, false
		}

		n.setEdge(key, newChild)
		indx := -1
		var notEmpty bool

		for edge, i := range n.children {
			if i != nil {
				if indx != -1 {
					notEmpty = true
					break
				} else {
					indx = edge
				}
			}
		}
		if indx != -1 && n.value != nil {
			// We have one children and value, set notEmpty to true
			notEmpty = true
		}
		if notEmpty {
			// The full node still has some other values
			return n, true
		}
		if indx == -1 {
			// There are no children nodes
			if n.value == nil {
				// Everything is empty, return nil
				return nil, true
			}
			// The value is the only left, return a short node with it
			return &ShortNode{key: []byte{0x10}, child: n.value}, true
		}

		// Only one value left at indx
		nc := n.children[indx]

		obj, ok := nc.(*ShortNode)
		if !ok {
			obj := &ShortNode{}
			obj.key = []byte{byte(indx)}
			obj.child = nc
			return obj, true
		}

		ncc := &ShortNode{}
		ncc.key = concat([]byte{byte(indx)}, obj.key)
		ncc.child = obj.child

		return ncc, true
	}

	panic("it should not happen")
}

func prefixLen(k1, k2 []byte) int {
	max := len(k1)
	if l := len(k2); l < max {
		max = l
	}
	var i int
	for i = 0; i < max; i++ {
		if k1[i] != k2[i] {
			break
		}
	}
	return i
}

func concat(a, b []byte) []byte {
	c := make([]byte, len(a)+len(b))
	copy(c, a)
	copy(c[len(a):], b)
	return c
}
