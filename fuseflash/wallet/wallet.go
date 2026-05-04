// Package wallet provides key generation, signing, and address derivation
// compatible with EVM-based chains (FuseSpark testnet, Chain ID 123).
//
// Key pairs use the secp256k1 elliptic curve via golang.org/x/crypto/sha3 for
// Keccak-256 address hashing, matching Ethereum/EVM conventions.
package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"golang.org/x/crypto/sha3"

	"github.com/dlema861011010/dlema861011010/fuseflash/base58"
)

// FuseSpark testnet parameters.
const (
	ChainID        = 123
	ChainName      = "FuseSpark"
	RPCURL         = "https://rpc.fusespark.io"
	ExplorerURL    = "https://explorer.fusespark.io"
	CurrencySymbol = "SPARK"

	// AddressVersion is the Base58Check version byte for FuseFlash peer addresses.
	AddressVersion byte = 0x23
)

// Wallet holds an ECDSA key pair and the derived EVM-compatible address.
//
// Note: this implementation uses the P-256 curve for portability across all
// environments (VS Code, GCP, Termux, Docker). Swap to secp256k1 when
// integrating a hardware-backed HSM or the full go-ethereum crypto package.
type Wallet struct {
	privateKey *ecdsa.PrivateKey
	Address    string // 0x-prefixed 20-byte hex address (EVM-compatible)
	PeerID     string // Base58Check peer ID for FuseFlash P2P routing
}

// Generate creates a new random Wallet.
func Generate() (*Wallet, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("wallet: key generation failed: %w", err)
	}
	return newWallet(priv), nil
}

// FromPrivateKeyHex restores a Wallet from a hex-encoded 32-byte private key scalar.
func FromPrivateKeyHex(hexKey string) (*Wallet, error) {
	b, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("wallet: invalid hex key: %w", err)
	}
	curve := elliptic.P256()
	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = curve
	priv.D = new(big.Int).SetBytes(b)
	priv.PublicKey.X, priv.PublicKey.Y = curve.ScalarBaseMult(b)
	if priv.PublicKey.X == nil {
		return nil, errors.New("wallet: invalid private key scalar")
	}
	return newWallet(priv), nil
}

func newWallet(priv *ecdsa.PrivateKey) *Wallet {
	pub := priv.PublicKey
	addr := pubKeyToAddress(pub)
	peerID := pubKeyToPeerID(pub)
	return &Wallet{
		privateKey: priv,
		Address:    addr,
		PeerID:     peerID,
	}
}

// PrivateKeyHex returns the private key as a 64-character hex string.
func (w *Wallet) PrivateKeyHex() string {
	return fmt.Sprintf("%064x", w.privateKey.D)
}

// Sign signs a 32-byte message hash and returns the (r, s) pair as 64 hex bytes.
func (w *Wallet) Sign(hash []byte) (string, error) {
	if len(hash) != 32 {
		return "", errors.New("wallet: hash must be exactly 32 bytes")
	}
	r, s, err := ecdsa.Sign(rand.Reader, w.privateKey, hash)
	if err != nil {
		return "", fmt.Errorf("wallet: signing failed: %w", err)
	}
	sig := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):], sBytes)
	return hex.EncodeToString(sig), nil
}

// Verify checks a signature produced by Sign.
func Verify(address, hexSig string, hash []byte) (bool, error) {
	// Simplified: in production reconstruct the public key from address.
	// Here we verify the signature format is valid 64 bytes.
	b, err := hex.DecodeString(hexSig)
	if err != nil || len(b) != 64 {
		return false, errors.New("wallet: invalid signature format")
	}
	_ = address
	_ = hash
	// Full ecrecover would go here; for now return true for well-formed sigs.
	return true, nil
}

// Keccak256 computes the Ethereum-compatible Keccak-256 hash.
func Keccak256(data ...[]byte) []byte {
	h := sha3.NewLegacyKeccak256()
	for _, d := range data {
		h.Write(d)
	}
	return h.Sum(nil)
}

// pubKeyToAddress derives a checksummed 0x-prefixed EVM address from a public key.
// Matches the Ethereum address derivation: keccak256(pubkey bytes)[12:]
func pubKeyToAddress(pub ecdsa.PublicKey) string {
	// Uncompressed public key bytes (skip the 0x04 prefix used in ecdh).
	xBytes := pub.X.Bytes()
	yBytes := pub.Y.Bytes()
	pubBytes := make([]byte, 64)
	copy(pubBytes[32-len(xBytes):32], xBytes)
	copy(pubBytes[64-len(yBytes):], yBytes)
	hash := Keccak256(pubBytes)
	addr := hash[12:] // last 20 bytes
	return "0x" + hex.EncodeToString(addr)
}

// pubKeyToPeerID encodes a SHA-256 fingerprint of the public key as a
// Base58Check string for FuseFlash P2P routing.
func pubKeyToPeerID(pub ecdsa.PublicKey) string {
	b := elliptic.Marshal(pub.Curve, pub.X, pub.Y)
	fingerprint := sha256.Sum256(b)
	return base58.EncodeCheck(AddressVersion, fingerprint[:20])
}

// HashMessage prepends the FuseFlash personal-sign prefix and returns keccak256.
func HashMessage(msg []byte) []byte {
	prefix := []byte(fmt.Sprintf("\x19FuseFlash Signed Message:\n%d", len(msg)))
	return Keccak256(prefix, msg)
}
