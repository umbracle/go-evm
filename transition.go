package state

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/evmc/v10/bindings/go/evmc"
	"github.com/umbracle/go-evm/evm"
)

const (
	spuriousDragonMaxCodeSize = 24576

	// Per transaction not creating a contract
	TxGas uint64 = 21000

	// Per transaction that creates a contract
	TxGasContractCreation uint64 = 53000
)

// getHashByNumber returns the hash function of a block number
type GetHashByNumber = func(i uint64) evmc.Hash

type Transition struct {
	// txn is the transaction of changes
	txn *Txn

	// parametrization of the transition
	config *Config
}

// TxContext is the context of the transaction
type TxContext struct {
	Hash       evmc.Hash
	GasPrice   evmc.Hash
	Origin     evmc.Address
	Coinbase   evmc.Address
	Number     int64
	Timestamp  int64
	GasLimit   int64
	ChainID    int64
	Difficulty evmc.Hash
}

// NewExecutor creates a new executor
func NewTransition(opts ...ConfigOption) *Transition {
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	txn := NewTxn(config.State)
	txn.rev = config.Rev

	transition := &Transition{
		config: config,
		txn:    txn,
	}
	return transition
}

func (e *Transition) Commit() []*Object {
	return e.txn.Commit()
}

func (t *Transition) Txn() *Txn {
	return t.txn
}

// Write writes another transaction to the executor
func (t *Transition) Write(msg *Message) (*Output, error) {
	output, err := t.applyImpl(msg)
	if err != nil {
		return nil, err
	}

	if t.isRevision(evmc.Byzantium) {
		// The suicided accounts are set as deleted for the next iteration
		t.txn.CleanDeleteObjects(true)
	} else {
		// TODO: If byzntium is enabled you need a special step to commit the data yourself
		t.txn.CleanDeleteObjects(t.isRevision(evmc.SpuriousDragon))
	}

	return output, nil
}

// Apply applies a new transaction
func (t *Transition) applyImpl(msg *Message) (*Output, error) {
	if err := t.preCheck(msg); err != nil {
		return nil, err
	}
	output := t.Apply(msg)
	t.postCheck(msg, output)
	return output, nil
}

func (t *Transition) isRevision(rev evmc.Revision) bool {
	return rev <= t.config.Rev
}

func (t *Transition) preCheck(msg *Message) error {
	// 1. the nonce of the message caller is correct
	nonce := t.txn.GetNonce(msg.From)
	if nonce != msg.Nonce {
		return fmt.Errorf("incorrect nonce")
	}

	// 2. deduct the upfront max gas cost to cover transaction fee(gaslimit * gasprice)
	upfrontGasCost := new(big.Int).Set(msg.GasPrice)
	upfrontGasCost.Mul(upfrontGasCost, new(big.Int).SetUint64(msg.Gas))

	err := t.txn.SubBalance(msg.From, upfrontGasCost)
	if err != nil {
		if err == errNotEnoughFunds {
			return fmt.Errorf("not enough funds to cover gas costs")
		}
		return err
	}

	// 4. there is no overflow when calculating intrinsic gas
	intrinsicGasCost, err := TransactionGasCost(msg, t.isRevision(evmc.Homestead), t.isRevision(evmc.Istanbul))
	if err != nil {
		return err
	}

	// 5. the purchased gas is enough to cover intrinsic usage
	gasLeft := msg.Gas - intrinsicGasCost
	// Because we are working with unsigned integers for gas, the `>` operator is used instead of the more intuitive `<`
	if gasLeft > msg.Gas {
		return fmt.Errorf("not enough gas supplied for intrinsic gas costs")
	}

	// 6. caller has enough balance to cover asset transfer for **topmost** call
	if balance := t.txn.GetBalance(msg.From); balance.Cmp(msg.Value) < 0 {
		return errNotEnoughFunds
	}

	msg.Gas = gasLeft
	return nil
}

func (t *Transition) postCheck(msg *Message, output *Output) {
	var gasUsed uint64

	intrinsicGasCost, _ := TransactionGasCost(msg, t.isRevision(evmc.Homestead), t.isRevision(evmc.Istanbul))
	msg.Gas += intrinsicGasCost

	// Update gas used depending on the refund.
	refund := t.txn.GetRefund()
	{
		gasUsed = msg.Gas - output.GasLeft
		maxRefund := gasUsed / 2
		// Refund can go up to half the gas used
		if refund > maxRefund {
			refund = maxRefund
		}

		output.GasLeft += refund
		gasUsed -= refund
	}

	gasPrice := new(big.Int).Set(msg.GasPrice)

	// refund the sender
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(output.GasLeft), gasPrice)
	t.txn.AddBalance(msg.From, remaining)

	// pay the coinbase for the transaction
	coinbaseFee := new(big.Int).Mul(new(big.Int).SetUint64(gasUsed), gasPrice)
	t.txn.AddBalance(t.config.Ctx.Coinbase, coinbaseFee)
}

func (t *Transition) Apply(msg *Message) *Output {
	gasPrice := new(big.Int).Set(msg.GasPrice)
	value := new(big.Int).Set(msg.Value)

	// Override the context and set the specific transaction fields
	t.config.Ctx.GasPrice = bytesToHash(gasPrice.Bytes())
	t.config.Ctx.Origin = msg.From

	var retValue []byte
	var gasLeft int64
	var err error

	if msg.IsContractCreation() {
		address := createAddress(msg.From, t.txn.GetNonce(msg.From))
		contract := NewContractCreation(0, msg.From, address, value, msg.Gas, msg.Input)
		retValue, gasLeft, _, err = t.applyCreate(contract)
	} else {
		t.txn.IncrNonce(msg.From)
		c := NewContractCall(0, msg.From, *msg.To, value, msg.Gas, msg.Input)
		retValue, gasLeft, _, err = t.applyCall(c, evmc.Call)
	}

	output := &Output{
		ReturnValue: retValue,
		Logs:        t.txn.Logs(),
		GasLeft:     uint64(gasLeft),
	}

	if err != nil {
		output.Success = false
	} else {
		output.Success = true
	}

	// if the transaction created a contract, store the creation address in the receipt.
	if msg.To == nil {
		output.ContractAddress = createAddress(msg.From, msg.Nonce)
	}

	return output
}

func (t *Transition) isPrecompiled(codeAddr evmc.Address) bool {
	if _, ok := precompiledContracts[codeAddr]; !ok {
		return false
	}

	// byzantium precompiles
	switch codeAddr {
	case addr5:
		fallthrough
	case addr6:
		fallthrough
	case addr7:
		fallthrough
	case addr8:
		return t.isRevision(evmc.Byzantium)
	}

	// istanbul precompiles
	switch codeAddr {
	case addr9:
		return t.isRevision(evmc.Istanbul)
	}

	return true
}

func (t *Transition) run(c *Contract) ([]byte, int64, error) {
	// try to run a cheatcode first
	for _, cheat := range t.config.Cheatcodes {
		if cheat.CanRun(c.CodeAddress) {
			cheat.Run(c.CodeAddress, c.Input)
			// Do not consume any gas with the cheat codes
			return nil, int64(c.Gas), nil
		}
	}
	if t.isPrecompiled(c.CodeAddress) {
		return runPrecompiled(c.CodeAddress, c.Input, c.Gas, t.config.Rev)
	}

	evm := evm.EVM{
		Host: t,
		Rev:  t.config.Rev,
	}
	return evm.Run(c.Type, c.Address, c.Caller, c.Value, c.Input, int64(c.Gas), c.Depth, c.Static, c.CodeAddress)
}

func (t *Transition) transfer(from, to evmc.Address, amount *big.Int) error {
	if amount == nil {
		return nil
	}

	if err := t.txn.SubBalance(from, amount); err != nil {
		return err
	}
	t.txn.AddBalance(to, amount)
	return nil
}

func (t *Transition) applyCall(c *Contract, callType evmc.CallKind) ([]byte, int64, evmc.Address, error) {
	snapshot := t.txn.Snapshot()
	t.txn.TouchAccount(c.Address)

	if callType == evmc.Call {
		// Transfers only allowed on calls
		if err := t.transfer(c.Caller, c.Address, c.Value); err != nil {
			return nil, int64(c.Gas), evmc.Address{}, err
		}
	}

	retValue, gasLeft, err := t.run(c)
	if err != nil {
		t.txn.RevertToSnapshot(snapshot)
		if err != evm.ErrExecutionReverted {
			// return value only allowed on error for reverted
			retValue = nil
		}
	}
	return retValue, gasLeft, evmc.Address{}, err
}

var emptyHash evmc.Hash

func (t *Transition) hasCodeOrNonce(addr evmc.Address) bool {
	nonce := t.txn.GetNonce(addr)
	if nonce != 0 {
		return true
	}
	codeHash := t.txn.GetCodeHash(addr)
	if codeHash != EmptyCodeHash && codeHash != emptyHash {
		return true
	}
	return false
}

func (t *Transition) applyCreate(c *Contract) ([]byte, int64, evmc.Address, error) {
	gasLimit := c.Gas

	var address evmc.Address
	if c.Type == evmc.Create {
		address = createAddress(c.Caller, t.GetNonce(c.Caller))
	} else if c.Type == evmc.Create2 {
		address = createAddress2(c.Caller, c.Salt, c.Input)
	} else {
		panic("X1")
	}

	c.CodeAddress = address
	c.Address = address

	// Increment the nonce of the caller
	t.txn.IncrNonce(c.Caller)

	// Check if there if there is a collision and the address already exists
	if t.hasCodeOrNonce(c.Address) {
		return nil, 0, address, errors.New("contract address collision")
	}

	// Take snapshot of the current state
	snapshot := t.txn.Snapshot()

	if t.isRevision(evmc.SpuriousDragon) {
		// Force the creation of the account
		t.txn.CreateAccount(c.Address)
		t.txn.IncrNonce(c.Address)
	}

	// Transfer the value
	if err := t.transfer(c.Caller, c.Address, c.Value); err != nil {
		return nil, int64(gasLimit), address, err
	}

	retValue, gasLeft, err := t.run(c)

	if err != nil {
		t.txn.RevertToSnapshot(snapshot)
		if err != evm.ErrExecutionReverted {
			retValue = nil
		}
		// there is only return value when its an execution reverted
		return retValue, gasLeft, address, err
	}

	if t.isRevision(evmc.SpuriousDragon) && len(retValue) > spuriousDragonMaxCodeSize {
		// Contract size exceeds 'SpuriousDragon' size limit
		t.txn.RevertToSnapshot(snapshot)
		return nil, 0, address, errors.New("evm: max code size exceeded")
	}

	gasCost := int64(len(retValue)) * 200

	if gasLeft < gasCost {
		err = errors.New("contract creation code storage out of gas")

		// Out of gas creating the contract
		if t.isRevision(evmc.Homestead) {
			t.txn.RevertToSnapshot(snapshot)
			gasLeft = 0
		}

		return nil, gasLeft, address, err
	}

	gasLeft -= gasCost
	t.txn.SetCode(c.Address, retValue)

	return nil, gasLeft, address, err
}

func (t *Transition) SetStorage(addr evmc.Address, key evmc.Hash, value evmc.Hash) evmc.StorageStatus {
	return t.txn.SetStorage(addr, key, value)
}

func (t *Transition) GetTxContext() evmc.TxContext {
	chainID := new(big.Int).SetInt64(t.config.Ctx.ChainID)
	cc := bytesToHash(chainID.Bytes())

	ctx := evmc.TxContext{
		GasPrice:   t.config.Ctx.GasPrice,
		Origin:     t.config.Ctx.Origin,
		Coinbase:   t.config.Ctx.Coinbase,
		Number:     t.config.Ctx.Number,
		Timestamp:  t.config.Ctx.Timestamp,
		GasLimit:   t.config.Ctx.GasLimit,
		Difficulty: t.config.Ctx.Difficulty,
		ChainID:    cc,
	}
	return ctx
}

func (t *Transition) GetBlockHash(number int64) (res evmc.Hash) {
	return t.config.GetHash(uint64(number))
}

func (t *Transition) EmitLog(addr evmc.Address, topics []evmc.Hash, data []byte) {
	t.txn.EmitLog(addr, topics, data)
}

func (t *Transition) GetCodeSize(addr evmc.Address) int {
	return t.txn.GetCodeSize(addr)
}

func (t *Transition) GetCodeHash(addr evmc.Address) (res evmc.Hash) {
	return t.txn.GetCodeHash(addr)
}

func (t *Transition) GetCode(addr evmc.Address) []byte {
	return t.txn.GetCode(addr)
}

func (t *Transition) GetBalance(addr evmc.Address) evmc.Hash {
	return bytesToHash(t.txn.GetBalance(addr).Bytes())
}

func (t *Transition) GetStorage(addr evmc.Address, key evmc.Hash) evmc.Hash {
	return t.txn.GetState(addr, key)
}

func (t *Transition) AccountExists(addr evmc.Address) bool {
	return t.txn.AccountExists(addr)
}

func (t *Transition) GetNonce(addr evmc.Address) uint64 {
	return t.txn.GetNonce(addr)
}

func (t *Transition) AccessAccount(addr evmc.Address) evmc.AccessStatus {
	panic("TODO")
}

func (t *Transition) AccessStorage(addr evmc.Address, key evmc.Hash) evmc.AccessStatus {
	panic("TODO")
}

func (t *Transition) Selfdestruct(addr evmc.Address, beneficiary evmc.Address) {
	if !t.txn.HasSuicided(addr) {
		t.txn.AddRefund(24000)
	}
	t.txn.AddBalance(beneficiary, t.txn.GetBalance(addr))
	t.txn.Suicide(addr)
}

func (t *Transition) Call(kind evmc.CallKind,
	recipient evmc.Address, sender evmc.Address, value evmc.Hash, input []byte, gas int64, depth int,
	static bool, salt evmc.Hash, codeAddress evmc.Address) ([]byte, int64, evmc.Address, error) {

	cc := &Contract{
		Type:        kind,
		Address:     recipient,
		Caller:      sender,
		CodeAddress: codeAddress,
		Depth:       depth,
		Value:       new(big.Int).SetBytes(value[:]),
		Input:       input,
		Gas:         uint64(gas),
		Static:      static,
		Salt:        salt,
	}
	retValue, gasLeft, addr, err := t.Callx(cc)
	return retValue, gasLeft, addr, err
}

func (t *Transition) Callx(c *Contract) ([]byte, int64, evmc.Address, error) {
	if c.Type == evmc.Create || c.Type == evmc.Create2 {
		return t.applyCreate(c)
	}
	return t.applyCall(c, c.Type)
}

func TransactionGasCost(msg *Message, isHomestead, isIstanbul bool) (uint64, error) {
	cost := uint64(0)

	// Contract creation is only paid on the homestead fork
	if msg.IsContractCreation() && isHomestead {
		cost += TxGasContractCreation
	} else {
		cost += TxGas
	}

	payload := msg.Input
	if len(payload) > 0 {
		zeros := uint64(0)
		for i := 0; i < len(payload); i++ {
			if payload[i] == 0 {
				zeros++
			}
		}

		nonZeros := uint64(len(payload)) - zeros
		nonZeroCost := uint64(68)
		if isIstanbul {
			nonZeroCost = 16
		}

		if (math.MaxUint64-cost)/nonZeroCost < nonZeros {
			return 0, fmt.Errorf("overflow in non-zeros intrinsic gas calculation")
		}

		cost += nonZeros * nonZeroCost

		if (math.MaxUint64-cost)/4 < zeros {
			return 0, fmt.Errorf("overflow in zeros intrinsic gas calculation")
		}

		cost += zeros * 4
	}

	return cost, nil
}
