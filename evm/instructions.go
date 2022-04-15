package evm

import (
	"math/big"
	"math/bits"
	"sync"

	"github.com/ethereum/evmc/v10/bindings/go/evmc"
	"github.com/umbracle/ethgo"
)

type instruction func(c *state)

var (
	zero     = big.NewInt(0)
	one      = big.NewInt(1)
	wordSize = big.NewInt(32)
)

func opAdd(c *state) {
	a := c.pop()
	b := c.top()

	b.Add(a, b)
	toU256(b)
}

func opMul(c *state) {
	a := c.pop()
	b := c.top()

	b.Mul(a, b)
	toU256(b)
}

func opSub(c *state) {
	a := c.pop()
	b := c.top()

	b.Sub(a, b)
	toU256(b)
}

func opDiv(c *state) {
	a := c.pop()
	b := c.top()

	if b.Sign() == 0 {
		// division by zero
		b.Set(zero)
	} else {
		b.Div(a, b)
		toU256(b)
	}
}

func opSDiv(c *state) {
	a := to256(c.pop())
	b := to256(c.top())

	if b.Sign() == 0 {
		// division by zero
		b.Set(zero)
	} else {
		neg := a.Sign() != b.Sign()
		b.Div(a.Abs(a), b.Abs(b))
		if neg {
			b.Neg(b)
		}
		toU256(b)
	}
}

func opMod(c *state) {
	a := c.pop()
	b := c.top()

	if b.Sign() == 0 {
		// division by zero
		b.Set(zero)
	} else {
		b.Mod(a, b)
		toU256(b)
	}
}

func opSMod(c *state) {
	a := to256(c.pop())
	b := to256(c.top())

	if b.Sign() == 0 {
		// division by zero
		b.Set(zero)
	} else {
		neg := a.Sign() < 0
		b.Mod(a.Abs(a), b.Abs(b))
		if neg {
			b.Neg(b)
		}
		toU256(b)
	}
}

var bigPool = sync.Pool{
	New: func() interface{} {
		return new(big.Int)
	},
}

func acquireBig() *big.Int {
	return bigPool.Get().(*big.Int)
}

func releaseBig(b *big.Int) {
	bigPool.Put(b)
}

func opExp(c *state) {
	x := c.pop()
	y := c.top()

	var gas uint64
	if c.isRevision(evmc.SpuriousDragon) {
		gas = 50
	} else {
		gas = 10
	}
	gasCost := uint64((y.BitLen()+7)/8) * gas
	if !c.consumeGas(gasCost) {
		return
	}

	z := acquireBig().Set(one)

	// https://www.programminglogic.com/fast-exponentiation-algorithms/
	for _, d := range y.Bits() {
		for i := 0; i < _W; i++ {
			if d&1 == 1 {
				toU256(z.Mul(z, x))
			}
			d >>= 1
			toU256(x.Mul(x, x))
		}
	}
	y.Set(z)
	releaseBig(z)
}

func opAddMod(c *state) {
	a := c.pop()
	b := c.pop()
	z := c.top()

	if z.Sign() == 0 {
		// divison by zero
		z.Set(zero)
	} else {
		a = a.Add(a, b)
		z = z.Mod(a, z)
		toU256(z)
	}
}

func opMulMod(c *state) {
	a := c.pop()
	b := c.pop()
	z := c.top()

	if z.Sign() == 0 {
		// divison by zero
		z.Set(zero)
	} else {
		a = a.Mul(a, b)
		z = z.Mod(a, z)
		toU256(z)
	}
}

func opAnd(c *state) {
	a := c.pop()
	b := c.top()

	b.And(a, b)
}

func opOr(c *state) {
	a := c.pop()
	b := c.top()

	b.Or(a, b)
}

func opXor(c *state) {
	a := c.pop()
	b := c.top()

	b.Xor(a, b)
}

var opByteMask = big.NewInt(255)

func opByte(c *state) {
	x := c.pop()
	y := c.top()

	indx := x.Int64()
	if indx > 31 {
		y.Set(zero)
	} else {
		sh := (31 - indx) * 8
		y.Rsh(y, uint(sh))
		y.And(y, opByteMask)
	}
}

func opNot(c *state) {
	a := c.top()

	a.Not(a)
	toU256(a)
}

func opIsZero(c *state) {
	a := c.top()

	if a.Sign() == 0 {
		a.Set(one)
	} else {
		a.Set(zero)
	}
}

func opEq(c *state) {
	a := c.pop()
	b := c.top()

	if a.Cmp(b) == 0 {
		b.Set(one)
	} else {
		b.Set(zero)
	}
}

func opLt(c *state) {
	a := c.pop()
	b := c.top()

	if a.Cmp(b) < 0 {
		b.Set(one)
	} else {
		b.Set(zero)
	}
}

func opGt(c *state) {
	a := c.pop()
	b := c.top()

	if a.Cmp(b) > 0 {
		b.Set(one)
	} else {
		b.Set(zero)
	}
}

func opSlt(c *state) {
	a := to256(c.pop())
	b := to256(c.top())

	if a.Cmp(b) < 0 {
		b.Set(one)
	} else {
		b.Set(zero)
	}
}

func opSgt(c *state) {
	a := to256(c.pop())
	b := to256(c.top())

	if a.Cmp(b) > 0 {
		b.Set(one)
	} else {
		b.Set(zero)
	}
}

func opSignExtension(c *state) {
	ext := c.pop()
	x := c.top()

	if ext.Cmp(wordSize) > 0 {
		return
	}
	if x == nil {
		return
	}

	bit := uint(ext.Uint64()*8 + 7)

	mask := acquireBig().Set(one)
	mask.Lsh(mask, bit)
	mask.Sub(mask, one)

	if x.Bit(int(bit)) > 0 {
		mask.Not(mask)
		x.Or(x, mask)
	} else {
		x.And(x, mask)
	}

	toU256(x)
	releaseBig(mask)
}

func equalOrOverflowsUint256(b *big.Int) bool {
	return b.BitLen() > 8
}

func opShl(c *state) {
	if !c.isRevision(evmc.Constantinople) {
		c.exit(errOpCodeNotFound)
		return
	}

	shift := c.pop()
	value := c.top()

	if equalOrOverflowsUint256(shift) {
		value.Set(zero)
	} else {
		value.Lsh(value, uint(shift.Uint64()))
		toU256(value)
	}
}

func opShr(c *state) {
	if !c.isRevision(evmc.Constantinople) {
		c.exit(errOpCodeNotFound)
		return
	}

	shift := c.pop()
	value := c.top()

	if equalOrOverflowsUint256(shift) {
		value.Set(zero)
	} else {
		value.Rsh(value, uint(shift.Uint64()))
		toU256(value)
	}
}

func opSar(c *state) {
	if !c.isRevision(evmc.Constantinople) {
		c.exit(errOpCodeNotFound)
		return
	}

	shift := c.pop()
	value := to256(c.top())

	if equalOrOverflowsUint256(shift) {
		if value.Sign() >= 0 {
			value.Set(zero)
		} else {
			value.Set(tt256m1)
		}
	} else {
		value.Rsh(value, uint(shift.Uint64()))
		toU256(value)
	}
}

// memory operations

var bufPool = sync.Pool{
	New: func() interface{} {
		// Store pointer to avoid heap allocation in caller
		// Please check SA6002 in StaticCheck for details
		buf := make([]byte, 128)
		return &buf
	},
}

func opMload(c *state) {
	offset := c.pop()

	var ok bool
	c.tmp, ok = c.get2(c.tmp[:0], offset, wordSize)
	if !ok {
		return
	}
	c.push1().SetBytes(c.tmp)
}

var (
	_W = bits.UintSize
	_S = _W / 8
)

func opMStore(c *state) {
	offset := c.pop()
	val := c.pop()

	if !c.checkMemory(offset, wordSize) {
		return
	}

	o := offset.Uint64()
	buf := c.memory[o : o+32]

	i := 32

	// convert big.int to bytes
	// https://golang.org/src/math/big/nat.go#L1284
	for _, d := range val.Bits() {
		for j := 0; j < _S; j++ {
			i--
			buf[i] = byte(d)
			d >>= 8
		}
	}

	// fill the rest of the slot with zeros
	for i > 0 {
		i--
		buf[i] = 0
	}
}

func opMStore8(c *state) {
	offset := c.pop()
	val := c.pop()

	if !c.checkMemory(offset, one) {
		return
	}
	c.memory[offset.Uint64()] = byte(val.Uint64() & 0xff)
}

// --- storage ---

func opSload(c *state) {
	loc := c.top()

	var gas uint64
	if c.isRevision(evmc.Istanbul) {
		// eip-1884
		gas = 800
	} else if c.isRevision(evmc.TangerineWhistle) {
		gas = 200
	} else {
		gas = 50
	}
	if !c.consumeGas(gas) {
		return
	}

	val := c.host.GetStorage(c.Address, bigToHash(loc))
	loc.SetBytes(val[:])
}

func opSStore(c *state) {
	if c.inStaticCall() {
		c.exit(errWriteProtection)
		return
	}

	if c.isRevision(evmc.Istanbul) && c.gas <= 2300 {
		c.exit(errOutOfGas)
		return
	}

	key := c.popHash()
	val := c.popHash()

	legacyGasMetering := !c.isRevision(evmc.Istanbul) && (c.isRevision(evmc.Petersburg) || !c.isRevision(evmc.Constantinople))

	status := c.host.SetStorage(c.Address, key, val)
	cost := uint64(0)

	switch status {
	case evmc.StorageUnchanged:
		if c.isRevision(evmc.Istanbul) {
			// eip-2200
			cost = 800
		} else if legacyGasMetering {
			cost = 5000
		} else {
			cost = 200
		}

	case evmc.StorageModified:
		cost = 5000

	case evmc.StorageModifiedAgain:
		if c.isRevision(evmc.Istanbul) {
			// eip-2200
			cost = 800
		} else if legacyGasMetering {
			cost = 5000
		} else {
			cost = 200
		}

	case evmc.StorageAdded:
		cost = 20000

	case evmc.StorageDeleted:
		cost = 5000
	}
	if !c.consumeGas(cost) {
		return
	}
}

const sha3WordGas uint64 = 6

func opSha3(c *state) {
	offset := c.pop()
	length := c.pop()

	var ok bool
	if c.tmp, ok = c.get2(c.tmp[:0], offset, length); !ok {
		return
	}

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * sha3WordGas) {
		return
	}

	v := c.push1()
	v.SetBytes(ethgo.Keccak256(c.tmp))
}

func opPop(c *state) {
	c.pop()
}

// context operations

func opAddress(c *state) {
	c.push1().SetBytes(c.Address[:])
}

func opBalance(c *state) {
	addr, _ := c.popAddr()

	var gas uint64
	if c.isRevision(evmc.Istanbul) {
		// eip-1884
		gas = 700
	} else if c.isRevision(evmc.TangerineWhistle) {
		gas = 400
	} else {
		gas = 20
	}

	if !c.consumeGas(gas) {
		return
	}

	balance := c.host.GetBalance(addr)
	c.push1().SetBytes(balance[:])
}

func opSelfBalance(c *state) {
	if !c.isRevision(evmc.Istanbul) {
		c.exit(errOpCodeNotFound)
		return
	}

	balance := c.host.GetBalance(c.Address)
	c.push1().SetBytes(balance[:])
}

func opChainID(c *state) {
	if !c.isRevision(evmc.Istanbul) {
		c.exit(errOpCodeNotFound)
		return
	}

	chainID := c.host.GetTxContext().ChainID
	c.push1().SetBytes(chainID[:])
}

func opOrigin(c *state) {
	origin := c.host.GetTxContext().Origin
	c.push1().SetBytes(origin[:])
}

func opCaller(c *state) {
	c.push1().SetBytes(c.Caller[:])
}

func opCallValue(c *state) {
	v := c.push1()
	if value := c.Value; value != nil {
		v.Set(value)
	} else {
		v.Set(zero)
	}
}

func min(i, j uint64) uint64 {
	if i < j {
		return i
	}
	return j
}

func opCallDataLoad(c *state) {
	offset := c.top()

	bufPtr := bufPool.Get().(*[]byte)
	buf := *bufPtr
	c.setBytes(buf[:32], c.Input, 32, offset)
	offset.SetBytes(buf[:32])
	bufPool.Put(bufPtr)
}

func opCallDataSize(c *state) {
	c.push1().SetUint64(uint64(len(c.Input)))
}

func opCodeSize(c *state) {
	c.push1().SetUint64(uint64(len(c.code)))
}

func opExtCodeSize(c *state) {
	addr, _ := c.popAddr()

	var gas uint64
	if c.isRevision(evmc.TangerineWhistle) {
		gas = 700
	} else {
		gas = 20
	}
	if !c.consumeGas(gas) {
		return
	}

	c.push1().SetUint64(uint64(c.host.GetCodeSize(addr)))
}

func opGasPrice(c *state) {
	gasPrice := c.host.GetTxContext().GasPrice
	c.push1().SetBytes(gasPrice[:])
}

func opReturnDataSize(c *state) {
	if !c.isRevision(evmc.Byzantium) {
		c.exit(errOpCodeNotFound)
	} else {
		c.push1().SetUint64(uint64(len(c.returnData)))
	}
}

func opExtCodeHash(c *state) {
	if !c.isRevision(evmc.Constantinople) {
		c.exit(errOpCodeNotFound)
		return
	}

	address, _ := c.popAddr()

	var gas uint64
	if c.isRevision(evmc.Istanbul) {
		gas = 700
	} else {
		gas = 400
	}
	if !c.consumeGas(gas) {
		return
	}

	v := c.push1()

	codeHash := c.host.GetCodeHash(address)
	v.SetBytes(codeHash[:])
}

func opPC(c *state) {
	c.push1().SetUint64(uint64(c.ip))
}

func opMSize(c *state) {
	c.push1().SetUint64(uint64(len(c.memory)))
}

func opGas(c *state) {
	c.push1().SetUint64(c.gas)
}

func (c *state) setBytes(dst, input []byte, size uint64, dataOffset *big.Int) {
	if !dataOffset.IsUint64() {
		// overflow, copy 'size' 0 bytes to dst
		for i := uint64(0); i < size; i++ {
			dst[i] = 0
		}
		return
	}

	inputSize := uint64(len(input))
	begin := min(dataOffset.Uint64(), inputSize)

	copySize := min(size, inputSize-begin)
	if copySize > 0 {
		copy(dst, input[begin:begin+copySize])
	}
	if size-copySize > 0 {
		dst = dst[copySize:]
		for i := uint64(0); i < size-copySize; i++ {
			dst[i] = 0
		}
	}
}

const copyGas uint64 = 3

func opExtCodeCopy(c *state) {
	address, _ := c.popAddr()
	memOffset := c.pop()
	codeOffset := c.pop()
	length := c.pop()

	if !c.checkMemory(memOffset, length) {
		return
	}

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * copyGas) {
		return
	}

	var gas uint64
	if c.isRevision(evmc.TangerineWhistle) {
		gas = 700
	} else {
		gas = 20
	}
	if !c.consumeGas(gas) {
		return
	}

	code := c.host.GetCode(address)
	if size != 0 {
		c.setBytes(c.memory[memOffset.Uint64():], code, size, codeOffset)
	}
}

func opCallDataCopy(c *state) {
	memOffset := c.pop()
	dataOffset := c.pop()
	length := c.pop()

	if !c.checkMemory(memOffset, length) {
		return
	}

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * copyGas) {
		return
	}

	if size != 0 {
		c.setBytes(c.memory[memOffset.Uint64():], c.Input, size, dataOffset)
	}
}

func opReturnDataCopy(c *state) {
	if !c.isRevision(evmc.Byzantium) {
		c.exit(errOpCodeNotFound)
		return
	}

	memOffset := c.pop()
	dataOffset := c.pop()
	length := c.pop()

	if !c.checkMemory(memOffset, length) {
		return
	}

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * copyGas) {
		return
	}

	end := length.Add(dataOffset, length)
	if !end.IsUint64() {
		c.exit(errReturnDataOutOfBounds)
		return
	}
	size = end.Uint64()
	if uint64(len(c.returnData)) < size {
		c.exit(errReturnDataOutOfBounds)
		return
	}

	data := c.returnData[dataOffset.Uint64():size]
	copy(c.memory[memOffset.Uint64():], data)
}

func opCodeCopy(c *state) {
	memOffset := c.pop()
	dataOffset := c.pop()
	length := c.pop()

	if !c.checkMemory(memOffset, length) {
		return
	}

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * copyGas) {
		return
	}
	if size != 0 {
		c.setBytes(c.memory[memOffset.Uint64():], c.code, size, dataOffset)
	}
}

// block information

func opBlockHash(c *state) {
	num := c.top()

	if !num.IsInt64() {
		num.Set(zero)
		return
	}

	n := num.Int64()
	lastBlock := c.host.GetTxContext().Number

	if lastBlock-257 < n && n < lastBlock {
		blockHash := c.host.GetBlockHash(n)
		num.SetBytes(blockHash[:])
	} else {
		num.Set(zero)
	}
}

func opCoinbase(c *state) {
	coinbase := c.host.GetTxContext().Coinbase
	c.push1().SetBytes(coinbase[:])
}

func opTimestamp(c *state) {
	c.push1().SetInt64(c.host.GetTxContext().Timestamp)
}

func opNumber(c *state) {
	c.push1().SetInt64(c.host.GetTxContext().Number)
}

func opDifficulty(c *state) {
	diff := c.host.GetTxContext().Difficulty
	c.push1().SetBytes(diff[:])
}

func opGasLimit(c *state) {
	c.push1().SetInt64(c.host.GetTxContext().GasLimit)
}

var zeroBalance = evmc.Hash{}

func opSelfDestruct(c *state) {
	if c.inStaticCall() {
		c.exit(errWriteProtection)
		return
	}

	address, _ := c.popAddr()

	// try to remove the gas first
	var gas uint64

	if c.rev >= evmc.TangerineWhistle {
		gas = 5000
		if c.rev == evmc.TangerineWhistle || c.host.GetBalance(c.Address) != zeroBalance {
			if !c.host.AccountExists(address) {
				gas += 25000
			}
		}
	}
	if !c.consumeGas(gas) {
		return
	}

	c.host.Selfdestruct(c.Address, address)
	c.halt()
}

func opJump(c *state) {
	dest := c.pop()

	if c.validJumpdest(dest) {
		c.ip = int(dest.Uint64() - 1)
	} else {
		c.exit(errInvalidJump)
	}
}

func opJumpi(c *state) {
	dest := c.pop()
	cond := c.pop()

	if cond.Sign() != 0 {
		if c.validJumpdest(dest) {
			c.ip = int(dest.Uint64() - 1)
		} else {
			c.exit(errInvalidJump)
		}
	}
}

func opJumpDest(c *state) {
}

func opPush(n int) instruction {
	return func(c *state) {
		ins := c.code
		ip := c.ip

		v := c.push1()
		if ip+1+n > len(ins) {
			v.SetBytes(append(ins[ip+1:], make([]byte, n)...))
		} else {
			v.SetBytes(ins[ip+1 : ip+1+n])
		}

		c.ip += n
	}
}

func opDup(n int) instruction {
	return func(c *state) {
		if !c.stackAtLeast(n) {
			c.exit(errStackUnderflow)
		} else {
			val := c.peekAt(n)
			c.push1().Set(val)
		}
	}
}

func opSwap(n int) instruction {
	return func(c *state) {
		if !c.stackAtLeast(n + 1) {
			c.exit(errStackUnderflow)
		} else {
			c.swap(n)
		}
	}
}

func opLog(size int) instruction {
	size = size - 1
	return func(c *state) {
		if c.inStaticCall() {
			c.exit(errWriteProtection)
			return
		}

		if !c.stackAtLeast(2 + size) {
			c.exit(errStackUnderflow)
			return
		}

		mStart := c.pop()
		mSize := c.pop()

		topics := make([]evmc.Hash, size)
		for i := 0; i < size; i++ {
			topics[i] = bigToHash(c.pop())
		}

		var ok bool
		c.tmp, ok = c.get2(c.tmp[:0], mStart, mSize)
		if !ok {
			return
		}

		c.host.EmitLog(c.Address, topics, c.tmp)

		if !c.consumeGas(uint64(size) * 375) {
			return
		}
		if !c.consumeGas(mSize.Uint64() * 8) {
			return
		}
	}
}

func opStop(c *state) {
	c.halt()
}

func (c *state) getBalance(addr evmc.Address) *big.Int {
	raw := c.host.GetBalance(addr)
	return new(big.Int).SetBytes(raw[:])
}

func opCreate(op OpCode) instruction {
	return func(c *state) {
		if c.inStaticCall() {
			c.exit(errWriteProtection)
			return
		}

		if op == CREATE2 {
			if !c.isRevision(evmc.Constantinople) {
				c.exit(errOpCodeNotFound)
				return
			}
		}

		// reset the return data
		c.resetReturnData()

		// Pop input arguments
		value := c.pop()
		offset := c.pop()
		length := c.pop()

		var salt *big.Int
		if op == CREATE2 {
			salt = c.pop()
		}

		// check if the value can be transfered
		hasTransfer := value != nil && value.Sign() != 0

		// Both CREATE and CREATE2 use memory
		var input []byte
		var ok bool

		input, ok = c.get2(input[:0], offset, length) // Does the memory check
		if !ok {
			c.push1().Set(zero)
			return
		}

		if hasTransfer {
			if c.getBalance(c.Address).Cmp(value) < 0 {
				c.push1().Set(zero)
				return
			}
		}

		if op == CREATE2 {
			// Consume sha3 gas cost
			size := length.Uint64()
			if !c.consumeGas(((size + 31) / 32) * sha3WordGas) {
				c.push1().Set(zero)
				return
			}
		}

		// Calculate and consume gas for the call
		gas := c.gas

		// CREATE2 uses by default EIP150
		if c.isRevision(evmc.TangerineWhistle) || op == CREATE2 {
			gas -= gas / 64
		}

		if !c.consumeGas(gas) {
			c.push1().Set(zero)
			return
		}

		if c.Depth >= int(1024) {
			c.push1().Set(zero)
			c.gas += gas
			return
		}

		callType := opCodeToCallKind(op)

		var saltHash evmc.Hash
		if op == CREATE2 {
			saltHash = bigToHash(salt)
		}
		var valueHash evmc.Hash
		if value != nil {
			valueHash = bigToHash(value)
		}
		retValue, gasLeft, codeAddress, err := c.host.Call(callType, evmc.Address{}, c.Address, valueHash, input, int64(gas), c.Depth+1, false, saltHash, evmc.Address{})

		v := c.push1()
		if err != nil {
			v.Set(zero)
		} else {
			v.SetBytes(codeAddress[:])
		}

		c.gas += uint64(gasLeft)
		c.returnData = append(c.returnData[:0], retValue...)
	}
}

func opCodeToCallKind(op OpCode) evmc.CallKind {
	switch op {
	case CALL:
		return evmc.Call
	case STATICCALL:
		return evmc.Call
	case CALLCODE:
		return evmc.CallCode
	case DELEGATECALL:
		return evmc.DelegateCall
	case CREATE:
		return evmc.Create
	case CREATE2:
		return evmc.Create2
	default:
		panic("BUG")
	}
}

func opCall(op OpCode) instruction {
	return func(c *state) {
		c.resetReturnData()

		if op == CALL && c.inStaticCall() {
			if val := c.peekAt(3); val != nil && val.BitLen() > 0 {
				c.exit(errWriteProtection)
				return
			}
		}

		if op == DELEGATECALL && !c.isRevision(evmc.Homestead) {
			c.exit(errOpCodeNotFound)
			return
		}
		if op == STATICCALL && !c.isRevision(evmc.Byzantium) {
			c.exit(errOpCodeNotFound)
			return
		}

		callType := opCodeToCallKind(op)

		// Pop input arguments
		initialGas := c.pop()
		addr, _ := c.popAddr()

		var value *big.Int
		if op == CALL || op == CALLCODE {
			value = c.pop()
		}

		// input range
		inOffset := c.pop()
		inSize := c.pop()

		// output range
		retOffset := c.pop()
		retSize := c.pop()

		// Get the input arguments
		args, ok := c.get2(nil, inOffset, inSize)
		if !ok {
			return
		}
		// Check if the memory return offsets are out of bounds
		if !c.checkMemory(retOffset, retSize) {
			return
		}

		var gasCost uint64
		if c.isRevision(evmc.TangerineWhistle) {
			gasCost = 700
		} else {
			gasCost = 40
		}

		// isTangerine := c.isRevision(evmc.SpuriousDragon)
		transfersValue := (op == CALL || op == CALLCODE) && value != nil && value.Sign() != 0

		if op == CALL {
			if (transfersValue || c.rev < evmc.SpuriousDragon) && !c.host.AccountExists(addr) {
				gasCost += 25000
			}
		}
		if transfersValue {
			gasCost += 9000
		}

		var gas uint64

		ok = initialGas.IsUint64()
		if c.isRevision(evmc.TangerineWhistle) {
			availableGas := c.gas - gasCost
			availableGas = availableGas - availableGas/64

			if !ok || availableGas < initialGas.Uint64() {
				gas = availableGas
			} else {
				gas = initialGas.Uint64()
			}
		} else {
			if !ok {
				c.exit(errOutOfGas)
				return
			}
			gas = initialGas.Uint64()
		}

		gasCost = gasCost + gas

		// Consume gas cost
		if !c.consumeGas(gasCost) {
			return
		}
		if transfersValue {
			gas += 2300
		}

		caller := c.Address
		isStatic := false

		to := addr
		codeAddress := addr

		if op == STATICCALL || c.Static {
			isStatic = true
		}
		if op == CALLCODE || op == DELEGATECALL {
			to = c.Address
			if op == DELEGATECALL {
				value = c.Value
				caller = c.Caller
			}
		}

		if transfersValue {
			if c.getBalance(c.Address).Cmp(value) < 0 {
				c.gas += gas
				c.push1().Set(zero)
				return
			}
		}

		offset := retOffset.Uint64()
		size := retSize.Uint64()

		if c.Depth >= int(1024) {
			c.push1().Set(zero)
			c.gas += gas
			return
		}

		var valueHash evmc.Hash
		if value != nil {
			valueHash = bigToHash(value)
		}
		retValue, gasLeft, _, err := c.host.Call(callType, to, caller, valueHash, args, int64(gas), c.Depth+1, isStatic, [32]byte{}, codeAddress)

		v := c.push1()
		if err != nil {
			v.Set(zero)
		} else {
			v.Set(one)
		}

		if len(retValue) != 0 {
			copy(c.memory[offset:offset+size], retValue)
		}

		c.gas += uint64(gasLeft)
		c.returnData = append(c.returnData[:0], retValue...)
	}
}

func opHalt(op OpCode) instruction {
	return func(c *state) {
		if op == REVERT && !c.isRevision(evmc.Byzantium) {
			c.exit(errOpCodeNotFound)
			return
		}

		offset := c.pop()
		size := c.pop()

		var ok bool
		c.ret, ok = c.get2(c.ret[:0], offset, size)
		if !ok {
			return
		}

		if op == REVERT {
			c.exit(ErrExecutionReverted)
		} else {
			c.halt()
		}
	}
}

var (
	tt256   = new(big.Int).Lsh(big.NewInt(1), 256)   // 2 ** 256
	tt256m1 = new(big.Int).Sub(tt256, big.NewInt(1)) // 2 ** 256 - 1
)

func toU256(x *big.Int) *big.Int {
	if x.Sign() < 0 || x.BitLen() > 256 {
		x.And(x, tt256m1)
	}
	return x
}

func to256(x *big.Int) *big.Int {
	if x.BitLen() > 255 {
		x.Sub(x, tt256)
	}
	return x
}
