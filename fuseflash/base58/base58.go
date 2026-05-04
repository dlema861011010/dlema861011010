// Package base58 implements Base58Check encoding used for FuseFlash peer IDs.
// The alphabet mirrors Bitcoin's Base58 to stay compatible with common tooling.
package base58

import (
	"crypto/sha256"
	"errors"
	"math/big"
)

const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var (
	bigZero  = big.NewInt(0)
	bigRadix = big.NewInt(58)

	ErrChecksum  = errors.New("base58: checksum mismatch")
	ErrInvalidChar = errors.New("base58: invalid character")
)

// Encode encodes a byte slice to a Base58 string.
func Encode(input []byte) string {
	x := new(big.Int).SetBytes(input)

	result := make([]byte, 0, len(input)*136/100)
	mod := new(big.Int)
	for x.Cmp(bigZero) > 0 {
		x.DivMod(x, bigRadix, mod)
		result = append(result, alphabet[mod.Int64()])
	}

	// Add '1' for each leading zero byte.
	for _, b := range input {
		if b != 0x00 {
			break
		}
		result = append(result, alphabet[0])
	}

	// Reverse.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return string(result)
}

// Decode decodes a Base58 string to bytes. Returns ErrInvalidChar on bad input.
func Decode(input string) ([]byte, error) {
	x := new(big.Int)
	for _, c := range input {
		idx := -1
		for i, a := range alphabet {
			if a == c {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, ErrInvalidChar
		}
		x.Mul(x, bigRadix)
		x.Add(x, big.NewInt(int64(idx)))
	}

	decoded := x.Bytes()

	// Re-add leading zero bytes.
	leadingZeros := 0
	for _, c := range input {
		if c != rune(alphabet[0]) {
			break
		}
		leadingZeros++
	}

	result := make([]byte, leadingZeros+len(decoded))
	copy(result[leadingZeros:], decoded)
	return result, nil
}

// checksum computes the double-SHA256 checksum (first 4 bytes).
func checksum(data []byte) [4]byte {
	h1 := sha256.Sum256(data)
	h2 := sha256.Sum256(h1[:])
	var out [4]byte
	copy(out[:], h2[:4])
	return out
}

// EncodeCheck encodes data with a 4-byte checksum appended (Base58Check).
func EncodeCheck(version byte, payload []byte) string {
	b := make([]byte, 1+len(payload))
	b[0] = version
	copy(b[1:], payload)
	ck := checksum(b)
	full := append(b, ck[:]...)
	return Encode(full)
}

// DecodeCheck decodes a Base58Check string, verifies checksum, and returns
// the version byte and payload. Returns ErrChecksum when verification fails.
func DecodeCheck(input string) (version byte, payload []byte, err error) {
	decoded, err := Decode(input)
	if err != nil {
		return 0, nil, err
	}
	if len(decoded) < 5 {
		return 0, nil, ErrChecksum
	}
	body := decoded[:len(decoded)-4]
	given := decoded[len(decoded)-4:]
	ck := checksum(body)
	if ck[0] != given[0] || ck[1] != given[1] || ck[2] != given[2] || ck[3] != given[3] {
		return 0, nil, ErrChecksum
	}
	return body[0], body[1:], nil
}
