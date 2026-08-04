package txpool

import (
	"math/big"
	"testing"
)

// makeAddr returns a deterministic Address for testing.
func makeAddr(b byte) Address {
	var a Address
	a[19] = b
	return a
}

// makeTx creates a minimal valid transaction for testing.
func makeTx(nonce uint64, value int64) *Transaction {
	to := makeAddr(0x02)
	return &Transaction{
		Nonce:    nonce,
		From:     makeAddr(0x01),
		To:       &to,
		Value:    big.NewInt(value),
		GasLimit: 21000,
	}
}

func TestAddAndGet(t *testing.T) {
	pool := New()
	tx := makeTx(0, 1000)
	if err := pool.Add(tx); err != nil {
		t.Fatalf("Add: %v", err)
	}
	hash := tx.Hash()
	got, ok := pool.Get(hash)
	if !ok {
		t.Fatal("Get: transaction not found")
	}
	if got.Nonce != tx.Nonce {
		t.Errorf("Nonce: want %d, got %d", tx.Nonce, got.Nonce)
	}
}

func TestAddDuplicate(t *testing.T) {
	pool := New()
	tx := makeTx(1, 500)
	if err := pool.Add(tx); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	// Adding the same transaction again must be a no-op.
	if err := pool.Add(tx); err != nil {
		t.Fatalf("duplicate Add should be no-op: %v", err)
	}
	if pool.Len() != 1 {
		t.Errorf("Len: want 1, got %d", pool.Len())
	}
}

func TestRemove(t *testing.T) {
	pool := New()
	tx := makeTx(2, 200)
	pool.Add(tx) //nolint:errcheck
	hash := tx.Hash()
	pool.Remove(hash)
	if _, ok := pool.Get(hash); ok {
		t.Error("Remove: transaction still present after removal")
	}
}

func TestLen(t *testing.T) {
	pool := New()
	for i := uint64(0); i < 5; i++ {
		pool.Add(makeTx(i, int64(i+1)*100)) //nolint:errcheck
	}
	if pool.Len() != 5 {
		t.Errorf("Len: want 5, got %d", pool.Len())
	}
}

func TestPending(t *testing.T) {
	pool := New()
	tx1 := makeTx(0, 100)
	tx2 := makeTx(1, 200)
	pool.Add(tx1) //nolint:errcheck
	pool.Add(tx2) //nolint:errcheck

	pending := pool.Pending()
	if len(pending) != 2 {
		t.Errorf("Pending: want 2, got %d", len(pending))
	}
}

func TestFlush(t *testing.T) {
	pool := New()
	pool.Add(makeTx(0, 100)) //nolint:errcheck
	pool.Add(makeTx(1, 200)) //nolint:errcheck
	pool.Flush()
	if pool.Len() != 0 {
		t.Errorf("Flush: want 0 remaining, got %d", pool.Len())
	}
}

func TestValidate_NilValue(t *testing.T) {
	tx := &Transaction{
		From:     makeAddr(0x01),
		GasLimit: 21000,
	}
	if err := tx.Validate(); err == nil {
		t.Error("expected error for nil Value")
	}
}

func TestValidate_NegativeValue(t *testing.T) {
	tx := &Transaction{
		From:     makeAddr(0x01),
		Value:    big.NewInt(-1),
		GasLimit: 21000,
	}
	if err := tx.Validate(); err == nil {
		t.Error("expected error for negative Value")
	}
}

func TestValidate_ZeroGasLimit(t *testing.T) {
	tx := &Transaction{
		From:     makeAddr(0x01),
		Value:    big.NewInt(0),
		GasLimit: 0,
	}
	if err := tx.Validate(); err == nil {
		t.Error("expected error for zero GasLimit")
	}
}

func TestContractCreation(t *testing.T) {
	// A nil To field signals contract creation.
	tx := &Transaction{
		From:     makeAddr(0x01),
		To:       nil,
		Value:    big.NewInt(0),
		GasLimit: 50000,
		Data:     []byte{0x60, 0x60, 0x60, 0x40}, // dummy bytecode
	}
	p := New()
	if err := p.Add(tx); err != nil {
		t.Fatalf("Add contract-creation tx: %v", err)
	}
	// Verify the transaction is retrievable from the pool.
	hash := tx.Hash()
	if _, ok := p.Get(hash); !ok {
		t.Error("contract-creation transaction should be retrievable from pool")
	}
}

func TestHash_Deterministic(t *testing.T) {
	tx := makeTx(42, 9999)
	if tx.Hash() != tx.Hash() {
		t.Error("Hash is not deterministic")
	}
}

func TestHash_Unique(t *testing.T) {
	tx1 := makeTx(0, 100)
	tx2 := makeTx(1, 100) // different nonce
	if tx1.Hash() == tx2.Hash() {
		t.Error("different transactions should have different hashes")
	}
}

func TestGetMissing(t *testing.T) {
	pool := New()
	_, ok := pool.Get(ZeroHash)
	if ok {
		t.Error("Get on empty pool should return (nil, false)")
	}
}

func TestAddInvalidTx(t *testing.T) {
	pool := New()
	bad := &Transaction{
		From:     makeAddr(0x01),
		Value:    nil, // invalid
		GasLimit: 21000,
	}
	if err := pool.Add(bad); err == nil {
		t.Error("Add invalid transaction should return error")
	}
	if pool.Len() != 0 {
		t.Error("invalid transaction should not be added to pool")
	}
}
