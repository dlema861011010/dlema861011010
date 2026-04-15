// Package txpool implements a pending-transaction pool for the fuseflash node.
// Transactions are keyed by their SHA-256 hash, modelled after the go-ethereum
// mempool design but kept intentionally minimal.
package txpool

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sync"
)

// Address is a 20-byte Ethereum-style account address.
type Address [20]byte

// String returns the hex-encoded address with a leading "0x" prefix.
func (a Address) String() string { return "0x" + hex.EncodeToString(a[:]) }

// Hash is a 32-byte transaction/block hash.
type Hash [32]byte

// String returns the hex-encoded hash.
func (h Hash) String() string { return hex.EncodeToString(h[:]) }

// ZeroHash is the all-zero hash.
var ZeroHash Hash

// Transaction represents a single pending transaction.
type Transaction struct {
	Nonce    uint64
	From     Address
	To       *Address // nil for contract-creation transactions
	Value    *big.Int // amount in wei; must be non-nil and >= 0
	GasLimit uint64
	Data     []byte
}

// Hash computes the SHA-256 digest of the transaction's canonical encoding.
// The encoding covers all fields that affect the transaction's identity.
func (tx *Transaction) Hash() Hash {
	h := sha256.New()
	var buf [8]byte

	binary.BigEndian.PutUint64(buf[:], tx.Nonce)
	h.Write(buf[:])
	h.Write(tx.From[:])
	if tx.To != nil {
		h.Write(tx.To[:])
	} else {
		h.Write(make([]byte, 20)) // contract creation sentinel
	}
	var valueBytes []byte
	if tx.Value != nil {
		valueBytes = tx.Value.Bytes()
	}
	h.Write(valueBytes)
	binary.BigEndian.PutUint64(buf[:], tx.GasLimit)
	h.Write(buf[:])
	h.Write(tx.Data)

	var result Hash
	copy(result[:], h.Sum(nil))
	return result
}

// Validate checks that the transaction is structurally valid.
func (tx *Transaction) Validate() error {
	if tx.Value == nil {
		return errors.New("txpool: transaction value is nil")
	}
	if tx.Value.Sign() < 0 {
		return errors.New("txpool: transaction value is negative")
	}
	if tx.GasLimit == 0 {
		return errors.New("txpool: gas limit is zero")
	}
	return nil
}

// Pool holds pending transactions keyed by their hash.
type Pool struct {
	mu      sync.RWMutex
	pending map[Hash]*Transaction
}

// New creates an empty transaction pool.
func New() *Pool {
	return &Pool{
		pending: make(map[Hash]*Transaction),
	}
}

// Add validates a transaction and adds it to the pool.
// If a transaction with the same hash is already present the call is a no-op.
func (p *Pool) Add(tx *Transaction) error {
	if err := tx.Validate(); err != nil {
		return fmt.Errorf("txpool add: %w", err)
	}

	hash := tx.Hash()

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.pending[hash]; exists {
		return nil // already known
	}
	p.pending[hash] = tx
	return nil
}

// Get retrieves a transaction by its hash.
// It returns (tx, true) if found, or (nil, false) otherwise.
func (p *Pool) Get(hash Hash) (*Transaction, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	tx, ok := p.pending[hash]
	return tx, ok
}

// Remove deletes the transaction with the given hash from the pool.
// It is a no-op if the hash is not present.
func (p *Pool) Remove(hash Hash) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pending, hash)
}

// Pending returns a snapshot of all transactions currently in the pool.
func (p *Pool) Pending() []*Transaction {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*Transaction, 0, len(p.pending))
	for _, tx := range p.pending {
		out = append(out, tx)
	}
	return out
}

// Len returns the number of transactions currently in the pool.
func (p *Pool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.pending)
}

// Flush removes all transactions from the pool.
func (p *Pool) Flush() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending = make(map[Hash]*Transaction)
}
