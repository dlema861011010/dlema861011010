// Package base58 provides Base58 encoding and decoding for the fuseflash
// node, compatible with the Bitcoin/go-ethereum Base58 alphabet.
// It enables human-readable, compact representations of 32-byte node IDs
// and block hashes (e.g. "BA5KaFvZtWZvLzhn71twnC1e1uxGG8pJ7WzyfTbDLCoc").
package base58

import (
	"errors"
	"fmt"
	"math/big"
)

// alphabet is the standard Bitcoin Base58 character set.
const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// alphabetIdx maps each Base58 character to its numeric value.
var alphabetIdx [256]int8

func init() {
	for i := range alphabetIdx {
		alphabetIdx[i] = -1
	}
	for i, c := range alphabet {
		alphabetIdx[c] = int8(i)
	}
}

var bigZero = big.NewInt(0)
var big58 = big.NewInt(58)

// Encode encodes a byte slice into a Base58 string.
// Leading zero bytes are encoded as leading '1' characters.
func Encode(b []byte) string {
	// Count leading zero bytes.
	leadingZeros := 0
	for _, byt := range b {
		if byt != 0 {
			break
		}
		leadingZeros++
	}

	// Convert bytes to a big integer.
	n := new(big.Int).SetBytes(b)

	// Encode into Base58 characters.
	var result []byte
	mod := new(big.Int)
	for n.Cmp(bigZero) > 0 {
		n.DivMod(n, big58, mod)
		result = append(result, alphabet[mod.Int64()])
	}

	// Add leading '1' characters for each leading zero byte.
	for i := 0; i < leadingZeros; i++ {
		result = append(result, '1')
	}

	// Reverse the result.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}

// ErrInvalidCharacter is returned when a Base58 string contains a character
// outside the Base58 alphabet.
var ErrInvalidCharacter = errors.New("base58: invalid character")

// Decode decodes a Base58 string into a byte slice.
// It returns ErrInvalidCharacter (wrapped with the offending character) if the
// string contains characters outside the Base58 alphabet.
func Decode(s string) ([]byte, error) {
	if s == "" {
		return []byte{}, nil
	}

	// Count leading '1' characters (each encodes a leading zero byte).
	leadingZeros := 0
	for _, c := range s {
		if c != '1' {
			break
		}
		leadingZeros++
	}

	// Decode Base58 characters into a big integer.
	n := new(big.Int)
	for _, c := range s {
		idx := alphabetIdx[c]
		if idx < 0 {
			return nil, fmt.Errorf("base58 decode: invalid character %q: %w", c, ErrInvalidCharacter)
		}
		n.Mul(n, big58)
		n.Add(n, big.NewInt(int64(idx)))
	}

	// Convert the big integer to bytes.
	decoded := n.Bytes()

	// Prepend leading zero bytes.
	result := make([]byte, leadingZeros+len(decoded))
	copy(result[leadingZeros:], decoded)
	return result, nil
}

// DecodeFixed decodes a Base58 string and returns exactly size bytes.
// If the decoded value is shorter than size, it is left-padded with zero bytes.
// It returns an error only if the decoded value is larger than size.
func DecodeFixed(s string, size int) ([]byte, error) {
	b, err := Decode(s)
	if err != nil {
		return nil, err
	}
	if len(b) > size {
		return nil, fmt.Errorf("base58 decode: decoded %d bytes, want %d", len(b), size)
	}
	// Pad with leading zeros if needed.
	if len(b) < size {
		padded := make([]byte, size)
		copy(padded[size-len(b):], b)
		return padded, nil
	}
	return b, nil
}