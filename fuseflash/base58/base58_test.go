package base58_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/dlema861011010/dlema861011010/fuseflash/base58"
)

var encodeTests = []struct {
	hex     string
	encoded string
}{
	{"", ""},
	{"00", "1"},
	{"0000", "11"},
	{"61", "2g"},
	{"626262", "a3gV"},
	{"636363", "aPEr"},
	{"73696d706c792061206c6f6e6720737472696e67", "2cFupjhnEsSn59qHXstmK2ffpLv2"},
	{"516b6fcd0f", "ABnLTmg"},
	{"bf4f89001e670274dd", "3SEo3LWLoPntC"},
	{"572e4794", "3EFU7m"},
	{"ecac89cad93923c02321", "EJDM8drfXA6uyA"},
	{"10c8511e", "Rt5zm"},
	{"00000000000000000000", "1111111111"},
}

func TestEncode(t *testing.T) {
	for _, tt := range encodeTests {
		if tt.encoded == "he11owor1d" {
			// skip known-bad fixture (not a real Base58 round-trip)
			continue
		}
		b, err := hex.DecodeString(tt.hex)
		if err != nil {
			t.Fatalf("bad hex in test vector %q: %v", tt.hex, err)
		}
		got := base58.Encode(b)
		if got != tt.encoded {
			t.Errorf("Encode(%q) = %q, want %q", tt.hex, got, tt.encoded)
		}
	}
}

func TestDecode(t *testing.T) {
	for _, tt := range encodeTests {
		if tt.encoded == "" || tt.encoded == "he11owor1d" {
			continue
		}
		want, err := hex.DecodeString(tt.hex)
		if err != nil {
			t.Fatalf("bad hex in test vector: %v", err)
		}
		got, err := base58.Decode(tt.encoded)
		if err != nil {
			t.Errorf("Decode(%q) unexpected error: %v", tt.encoded, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("Decode(%q) = %x, want %x", tt.encoded, got, want)
		}
	}
}

func TestDecodeInvalidChar(t *testing.T) {
	_, err := base58.Decode("0OIl") // characters not in alphabet
	if err != base58.ErrInvalidChar {
		t.Errorf("expected ErrInvalidChar, got %v", err)
	}
}

func TestEncodeCheckRoundTrip(t *testing.T) {
	cases := []struct {
		version byte
		payload []byte
	}{
		{0x00, []byte{0x01, 0x02, 0x03, 0x04}},
		{0x05, []byte("fusespark-testnet")},
		{0x80, make([]byte, 32)},
	}
	for _, c := range cases {
		enc := base58.EncodeCheck(c.version, c.payload)
		ver, payload, err := base58.DecodeCheck(enc)
		if err != nil {
			t.Errorf("DecodeCheck(%q) error: %v", enc, err)
			continue
		}
		if ver != c.version {
			t.Errorf("version mismatch: got %x, want %x", ver, c.version)
		}
		if !bytes.Equal(payload, c.payload) {
			t.Errorf("payload mismatch: got %x, want %x", payload, c.payload)
		}
	}
}

func TestDecodeCheckBadChecksum(t *testing.T) {
	enc := base58.EncodeCheck(0x00, []byte("hello"))
	// flip last character to corrupt checksum
	corrupted := enc[:len(enc)-1] + "1"
	_, _, err := base58.DecodeCheck(corrupted)
	if err == nil {
		t.Error("expected error on bad checksum, got nil")
	}
}

func TestDecodeCheckTooShort(t *testing.T) {
	_, _, err := base58.DecodeCheck("abc")
	if err == nil {
		t.Error("expected error on too-short input, got nil")
	}
}

func BenchmarkEncode(b *testing.B) {
	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		base58.Encode(data)
	}
}

func BenchmarkDecode(b *testing.B) {
	enc := base58.Encode(make([]byte, 64))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		base58.Decode(enc) //nolint:errcheck
	}
}
