package blocksync

import (
	"testing"
	"time"
)

func genesisHeader() *Header {
	return &Header{
		Number:     0,
		ParentHash: ZeroHash,
		Timestamp:  time.Unix(1_700_000_000, 0),
		Difficulty: 1,
	}
}

func nextHeader(parent *Header) *Header {
	return &Header{
		Number:     parent.Number + 1,
		ParentHash: parent.Hash(),
		Timestamp:  parent.Timestamp.Add(12 * time.Second),
		Difficulty: parent.Difficulty,
	}
}

func TestGenesisInsert(t *testing.T) {
	c := NewChain()
	g := genesisHeader()
	if err := c.Insert(g); err != nil {
		t.Fatalf("Insert genesis: %v", err)
	}
	if c.Height() != 0 {
		t.Errorf("height: want 0, got %d", c.Height())
	}
	if c.Genesis() == nil {
		t.Error("genesis should not be nil")
	}
}

func TestChainGrowth(t *testing.T) {
	c := NewChain()
	g := genesisHeader()
	c.Insert(g) //nolint:errcheck

	headers := []*Header{g}
	for i := 1; i <= 5; i++ {
		h := nextHeader(headers[i-1])
		headers = append(headers, h)
		if err := c.Insert(h); err != nil {
			t.Fatalf("Insert block %d: %v", i, err)
		}
	}

	if c.Height() != 5 {
		t.Errorf("height: want 5, got %d", c.Height())
	}
}

func TestInsertDuplicate(t *testing.T) {
	c := NewChain()
	g := genesisHeader()
	c.Insert(g) //nolint:errcheck
	// Inserting the same genesis again should be a no-op.
	if err := c.Insert(g); err != nil {
		t.Errorf("duplicate insert should be no-op: %v", err)
	}
}

func TestInsertMissingParent(t *testing.T) {
	c := NewChain()
	g := genesisHeader()
	c.Insert(g) //nolint:errcheck

	orphan := &Header{
		Number:     10,
		ParentHash: ZeroHash, // unknown parent for non-genesis
		Timestamp:  time.Unix(1_700_000_100, 0),
		Difficulty: 1,
	}
	// Number > 0 with a parent hash not in the chain should fail.
	if err := c.Insert(orphan); err == nil {
		t.Error("expected error for unknown parent, got nil")
	}
}

func TestInsertZeroTimestamp(t *testing.T) {
	c := NewChain()
	bad := &Header{
		Number:     0,
		ParentHash: ZeroHash,
		Difficulty: 1,
		// Timestamp intentionally zero
	}
	if err := c.Insert(bad); err == nil {
		t.Error("expected error for zero timestamp")
	}
}

func TestGetByNumber(t *testing.T) {
	c := NewChain()
	g := genesisHeader()
	c.Insert(g) //nolint:errcheck
	h, ok := c.GetByNumber(0)
	if !ok {
		t.Fatal("GetByNumber(0) not found")
	}
	if h.Number != 0 {
		t.Errorf("want block 0, got %d", h.Number)
	}
}

func TestGetByHash(t *testing.T) {
	c := NewChain()
	g := genesisHeader()
	c.Insert(g) //nolint:errcheck
	hash := g.Hash()
	h, ok := c.GetByHash(hash)
	if !ok {
		t.Fatal("GetByHash not found")
	}
	if h.Number != 0 {
		t.Errorf("want block 0, got %d", h.Number)
	}
}

func TestHasBlock(t *testing.T) {
	c := NewChain()
	g := genesisHeader()
	c.Insert(g) //nolint:errcheck
	if !c.HasBlock(g.Hash()) {
		t.Error("HasBlock: genesis should be present")
	}
	if c.HasBlock(ZeroHash) {
		t.Error("HasBlock: zero hash should not be present")
	}
}

func TestSyncHeaders(t *testing.T) {
	c := NewChain()
	announced := 0
	syncer := NewSyncer(c, func(_ *Header) { announced++ })

	g := genesisHeader()
	h1 := nextHeader(g)
	h2 := nextHeader(h1)

	n, err := syncer.HandleHeaders([]*Header{g, h1, h2})
	if err != nil {
		t.Fatalf("HandleHeaders: %v", err)
	}
	if n != 3 {
		t.Errorf("accepted: want 3, got %d", n)
	}
	if c.Height() != 2 {
		t.Errorf("height: want 2, got %d", c.Height())
	}
	if announced != 1 {
		t.Errorf("announce calls: want 1 (new head after batch), got %d", announced)
	}
}

func TestHandleAnnounce(t *testing.T) {
	c := NewChain()
	g := genesisHeader()
	c.Insert(g) //nolint:errcheck
	syncer := NewSyncer(c, nil)

	// Peer at height 5 should trigger sync.
	if !syncer.HandleAnnounce(5, ZeroHash) {
		t.Error("should want sync when peer is ahead")
	}
	// Peer at height 0 should not.
	if syncer.HandleAnnounce(0, ZeroHash) {
		t.Error("should not want sync when peer is equal or behind")
	}
}

func TestHeaderHash_Deterministic(t *testing.T) {
	h := genesisHeader()
	if h.Hash() != h.Hash() {
		t.Error("hash is not deterministic")
	}
}

func TestHeaderHash_Unique(t *testing.T) {
	h1 := genesisHeader()
	h2 := genesisHeader()
	h2.Difficulty = 999
	if h1.Hash() == h2.Hash() {
		t.Error("different headers should have different hashes")
	}
}
