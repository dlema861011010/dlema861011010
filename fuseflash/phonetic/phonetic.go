// Package phonetic provides phonetic encoding and validation for fuseflash
// node identifiers and Ethereum-style addresses, using NATO phonetic alphabet
// word encoding for human-readable representation of binary node IDs.
package phonetic

import (
	"errors"
	"fmt"
	"strings"
)

// natoAlphabet maps nibble values (0–15) to NATO phonetic alphabet words.
var natoAlphabet = [16]string{
	"Alpha", "Bravo", "Charlie", "Delta",
	"Echo", "Foxtrot", "Golf", "Hotel",
	"India", "Juliet", "Kilo", "Lima",
	"Mike", "November", "Oscar", "Papa",
}

// natoIndex is the reverse mapping from NATO word (lower-cased) to nibble value.
var natoIndex map[string]byte

func init() {
	natoIndex = make(map[string]byte, 16)
	for i, word := range natoAlphabet {
		natoIndex[strings.ToLower(word)] = byte(i)
	}
}

// EncodeBytes encodes a byte slice as a space-separated sequence of NATO
// phonetic words, two words per byte (one per nibble).
func EncodeBytes(b []byte) string {
	words := make([]string, 0, len(b)*2)
	for _, by := range b {
		words = append(words, natoAlphabet[by>>4], natoAlphabet[by&0x0f])
	}
	return strings.Join(words, " ")
}

// DecodeBytes reverses EncodeBytes, recovering the original byte slice.
// It returns an error if the word count is odd or any word is unrecognised.
func DecodeBytes(s string) ([]byte, error) {
	if s == "" {
		return []byte{}, nil
	}
	parts := strings.Fields(s)
	if len(parts)%2 != 0 {
		return nil, fmt.Errorf("phonetic decode: odd number of words (%d)", len(parts))
	}
	out := make([]byte, len(parts)/2)
	for i := 0; i < len(parts); i += 2 {
		hi, ok := natoIndex[strings.ToLower(parts[i])]
		if !ok {
			return nil, fmt.Errorf("phonetic decode: unknown word %q", parts[i])
		}
		lo, ok := natoIndex[strings.ToLower(parts[i+1])]
		if !ok {
			return nil, fmt.Errorf("phonetic decode: unknown word %q", parts[i+1])
		}
		out[i/2] = (hi << 4) | lo
	}
	return out, nil
}

// Validate checks that a phonetic string produced by EncodeBytes is well-formed
// and contains the expected number of bytes when decoded.
// Pass expectedBytes == -1 to skip the length check.
func Validate(s string, expectedBytes int) error {
	decoded, err := DecodeBytes(s)
	if err != nil {
		return fmt.Errorf("phonetic validate: %w", err)
	}
	if expectedBytes >= 0 && len(decoded) != expectedBytes {
		return fmt.Errorf("phonetic validate: expected %d bytes, got %d",
			expectedBytes, len(decoded))
	}
	return nil
}

// ValidateNodeID validates the phonetic representation of a 32-byte node ID.
func ValidateNodeID(s string) error {
	return Validate(s, 32)
}

// ValidateAddress validates the phonetic representation of a 20-byte
// Ethereum-style address.
func ValidateAddress(s string) error {
	return Validate(s, 20)
}

// ErrEmptyInput is returned when an empty string is passed to a function that
// requires non-empty input.
var ErrEmptyInput = errors.New("phonetic: empty input")

// NormaliseWords normalises a phonetic string by trimming whitespace and
// converting each NATO word to title-case.
func NormaliseWords(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrEmptyInput
	}
	parts := strings.Fields(s)
	for i, w := range parts {
		lower := strings.ToLower(w)
		if _, ok := natoIndex[lower]; !ok {
			return "", fmt.Errorf("phonetic normalise: unknown word %q", w)
		}
		parts[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(parts, " "), nil
}
