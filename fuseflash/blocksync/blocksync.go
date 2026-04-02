// Package blocksync implements block header synchronisation for the fuseflash
// node. It maintains a canonical chain of block headers and allows peers to
// announce and exchange headers, similar to the eth/63 protocol in go-ethereum.
package blocksync

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Hash is a 32-byte block/transaction hash.
type Hash [32]byte

// String returns the hex-encoded hash.
func (h Hash) String() string { return hex.EncodeToString(h[:]) }

// ZeroHash is the all-zero hash, used as the parent of the genesis block.
var ZeroHash Hash

// Header represents a single block header in the chain.
type Header struct {
	Number     uint64
	ParentHash Hash
	StateRoot  Hash
	Timestamp  time.Time
	Difficulty uint64
}

// Hash computes the SHA-256 digest of the header's canonical encoding.
func (h *Header) Hash() Hash {
	buf := make([]byte, 0, 8+32+32+8+8)
	buf = binary.BigEndian.AppendUint64(buf, h.Number)
	buf = append(buf, h.ParentHash[:]...)
	buf = append(buf, h.StateRoot[:]...)
	buf = binary.BigEndian.AppendUint64(buf, uint64(h.Timestamp.Unix()))
	buf = binary.BigEndian.AppendUint64(buf, h.Difficulty)
	return Hash(sha256.Sum256(buf))
}

// Validate checks that the header is structurally valid.
func (h *Header) Validate() error {
	if h.Number > 0 && h.ParentHash == ZeroHash {
		return fmt.Errorf("block %d: non-genesis block has zero parent hash", h.Number)
	}
	if h.Timestamp.IsZero() {
		return fmt.Errorf("block %d: zero timestamp", h.Number)
	}
	return nil
}

// Chain holds a sequence of block headers indexed by number and hash.
type Chain struct {
	mu      sync.RWMutex
	byNum   map[uint64]*Header
	byHash  map[Hash]*Header
	head    *Header
	genesis *Header
}

// NewChain creates an empty chain.
func NewChain() *Chain {
	return &Chain{
		byNum:  make(map[uint64]*Header),
		byHash: make(map[Hash]*Header),
	}
}

// Genesis returns the genesis block header, or nil if the chain is empty.
func (c *Chain) Genesis() *Header {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.genesis
}

// Head returns the canonical head header.
func (c *Chain) Head() *Header {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.head
}

// Height returns the block number of the current head, or 0 for an empty chain.
func (c *Chain) Height() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.head == nil {
		return 0
	}
	return c.head.Number
}

// HasBlock reports whether the chain contains a block with the given hash.
func (c *Chain) HasBlock(h Hash) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.byHash[h]
	return ok
}

// GetByNumber returns the header at the given block number.
func (c *Chain) GetByNumber(n uint64) (*Header, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	h, ok := c.byNum[n]
	return h, ok
}

// GetByHash returns the header with the given hash.
func (c *Chain) GetByHash(h Hash) (*Header, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	hdr, ok := c.byHash[h]
	return hdr, ok
}

// Insert validates and appends a new header to the chain.
// For the genesis block (Number == 0) no parent is required.
// For subsequent blocks the parent must already be present.
func (c *Chain) Insert(hdr *Header) error {
	if err := hdr.Validate(); err != nil {
		return fmt.Errorf("insert block %d: %w", hdr.Number, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	hash := hdr.Hash()
	if _, dup := c.byHash[hash]; dup {
		return nil // already known
	}

	if hdr.Number == 0 {
		if c.genesis != nil {
			return errors.New("insert: chain already has a genesis block")
		}
		c.genesis = hdr
	} else {
		if _, ok := c.byHash[hdr.ParentHash]; !ok {
			return fmt.Errorf("insert block %d: unknown parent %s", hdr.Number, hdr.ParentHash)
		}
	}

	c.byNum[hdr.Number] = hdr
	c.byHash[hash] = hdr

	if c.head == nil || hdr.Number > c.head.Number {
		c.head = hdr
	}
	return nil
}

// Sync processes a slice of headers received from a peer, inserting any that
// extend the canonical chain. It returns the number of new headers accepted.
func (c *Chain) Sync(headers []*Header) (int, error) {
	accepted := 0
	for _, h := range headers {
		if err := c.Insert(h); err != nil {
			return accepted, fmt.Errorf("sync: %w", err)
		}
		accepted++
	}
	return accepted, nil
}

// Announcer is a function type that can be called to announce the current head
// to a remote peer. Implementations are expected to be non-blocking.
type Announcer func(head *Header)

// Syncer coordinates header synchronisation with a single remote peer.
type Syncer struct {
	chain    *Chain
	announce Announcer
}

// NewSyncer creates a Syncer that uses the given chain and announce function.
func NewSyncer(chain *Chain, announce Announcer) *Syncer {
	return &Syncer{chain: chain, announce: announce}
}

// HandleAnnounce is called when a remote peer announces its head hash and
// height. If the peer is ahead, it returns true to signal that headers should
// be fetched.
func (s *Syncer) HandleAnnounce(peerHeight uint64, _ Hash) bool {
	return peerHeight > s.chain.Height()
}

// HandleHeaders processes a batch of headers delivered by a peer.
func (s *Syncer) HandleHeaders(headers []*Header) (int, error) {
	n, err := s.chain.Sync(headers)
	if err != nil {
		return n, err
	}
	if s.announce != nil && s.chain.Head() != nil {
		s.announce(s.chain.Head())
	}
	return n, nil
}
