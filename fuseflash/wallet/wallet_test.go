package wallet_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/dlema861011010/dlema861011010/fuseflash/wallet"
)

func TestGenerate(t *testing.T) {
	w, err := wallet.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.HasPrefix(w.Address, "0x") {
		t.Errorf("Address should start with 0x, got %q", w.Address)
	}
	if len(w.Address) != 42 {
		t.Errorf("Address should be 42 chars, got %d", len(w.Address))
	}
	if w.PeerID == "" {
		t.Error("PeerID should not be empty")
	}
}

func TestRoundTrip(t *testing.T) {
	w1, err := wallet.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	w2, err := wallet.FromPrivateKeyHex(w1.PrivateKeyHex())
	if err != nil {
		t.Fatalf("FromPrivateKeyHex() error: %v", err)
	}
	if w1.Address != w2.Address {
		t.Errorf("Address mismatch after round-trip: %q vs %q", w1.Address, w2.Address)
	}
	if w1.PeerID != w2.PeerID {
		t.Errorf("PeerID mismatch after round-trip: %q vs %q", w1.PeerID, w2.PeerID)
	}
}

func TestSign(t *testing.T) {
	w, _ := wallet.Generate()
	hash := wallet.Keccak256([]byte("hello fusespark"))
	sig, err := w.Sign(hash)
	if err != nil {
		t.Fatalf("Sign() error: %v", err)
	}
	b, err := hex.DecodeString(sig)
	if err != nil {
		t.Fatalf("signature not valid hex: %v", err)
	}
	if len(b) != 64 {
		t.Errorf("signature should be 64 bytes, got %d", len(b))
	}
}

func TestSignWrongHashLen(t *testing.T) {
	w, _ := wallet.Generate()
	_, err := w.Sign([]byte("tooshort"))
	if err == nil {
		t.Error("expected error for hash != 32 bytes")
	}
}

func TestHashMessage(t *testing.T) {
	h1 := wallet.HashMessage([]byte("test"))
	h2 := wallet.HashMessage([]byte("test"))
	if string(h1) != string(h2) {
		t.Error("HashMessage should be deterministic")
	}
	h3 := wallet.HashMessage([]byte("other"))
	if string(h1) == string(h3) {
		t.Error("HashMessage should differ for different inputs")
	}
}

func TestKeccak256(t *testing.T) {
	// Empty input keccak256 is a known constant.
	got := hex.EncodeToString(wallet.Keccak256([]byte{}))
	want := "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"
	if got != want {
		t.Errorf("Keccak256([]) = %s, want %s", got, want)
	}
}
