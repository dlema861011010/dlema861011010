package base58

import (
	"bytes"
	"errors"
	"testing"
)

// knownVector is the problem-statement test vector:
// the Base58 encoding of a 32-byte value.
const knownVector = "BA5KaFvZtWZvLzhn71twnC1e1uxGG8pJ7WzyfTbDLCoc"

var knownVectorBytes = []byte{
	0x96, 0xe7, 0xfb, 0x40, 0x0c, 0x2a, 0xd7, 0xec,
	0x15, 0x90, 0x44, 0x4b, 0xd7, 0x2e, 0xa1, 0x8e,
	0x71, 0xc8, 0xc5, 0x47, 0x51, 0x53, 0x31, 0xad,
	0x69, 0x97, 0xac, 0x2d, 0xa9, 0x95, 0x2f, 0x93,
}

func TestEncode_KnownVector(t *testing.T) {
	got := Encode(knownVectorBytes)
	if got != knownVector {
		t.Errorf("Encode: got %q, want %q", got, knownVector)
	}
}

func TestDecode_KnownVector(t *testing.T) {
	got, err := Decode(knownVector)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(got, knownVectorBytes) {
		t.Errorf("Decode: got %x, want %x", got, knownVectorBytes)
	}
}

func TestEncodeDecode_RoundTrip(t *testing.T) {
	cases := [][]byte{
		{},
		{0x00},
		{0x00, 0x00, 0x01},
		{0xff, 0xfe, 0xfd},
		{0x01, 0x02, 0x03, 0x04, 0x05},
		knownVectorBytes,
	}
	for _, tc := range cases {
		enc := Encode(tc)
		dec, err := Decode(enc)
		if err != nil {
			t.Errorf("Decode(%q): %v", enc, err)
			continue
		}
		// Normalise nil vs empty.
		if len(dec) == 0 && len(tc) == 0 {
			continue
		}
		if !bytes.Equal(dec, tc) {
			t.Errorf("round-trip mismatch: input %x, got %x", tc, dec)
		}
	}
}

func TestDecode_InvalidCharacter(t *testing.T) {
	_, err := Decode("0OIl") // characters not in the Base58 alphabet
	if err == nil {
		t.Error("expected error for invalid character, got nil")
	}
	if !errors.Is(err, ErrInvalidCharacter) {
		t.Errorf("expected ErrInvalidCharacter, got: %v", err)
	}
}

func TestDecode_MultiByteCharacter(t *testing.T) {
	// Multi-byte UTF-8 characters (rune >= 256) must return an error, not panic.
	_, err := Decode("abc🌍xyz")
	if err == nil {
		t.Error("expected error for multi-byte character, got nil")
	}
	if !errors.Is(err, ErrInvalidCharacter) {
		t.Errorf("expected ErrInvalidCharacter, got: %v", err)
	}
}

func TestDecode_EmptyString(t *testing.T) {
	got, err := Decode("")
	if err != nil {
		t.Fatalf("Decode empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Decode empty: want empty, got %x", got)
	}
}

func TestDecodeFixed_CorrectSize(t *testing.T) {
	got, err := DecodeFixed(knownVector, 32)
	if err != nil {
		t.Fatalf("DecodeFixed: %v", err)
	}
	if len(got) != 32 {
		t.Errorf("DecodeFixed: want 32 bytes, got %d", len(got))
	}
	if !bytes.Equal(got, knownVectorBytes) {
		t.Errorf("DecodeFixed: got %x, want %x", got, knownVectorBytes)
	}
}

func TestDecodeFixed_TooLarge(t *testing.T) {
	// Encoding of more than 4 bytes decoded into 4-byte target should fail.
	long := Encode([]byte{0x01, 0x02, 0x03, 0x04, 0x05})
	_, err := DecodeFixed(long, 4)
	if err == nil {
		t.Error("expected error when decoded bytes exceed size")
	}
}

func TestDecodeFixed_PadsLeadingZeros(t *testing.T) {
	// Single non-zero byte encoded as Base58 should pad to 4 bytes.
	enc := Encode([]byte{0x01})
	got, err := DecodeFixed(enc, 4)
	if err != nil {
		t.Fatalf("DecodeFixed pad: %v", err)
	}
	want := []byte{0x00, 0x00, 0x00, 0x01}
	if !bytes.Equal(got, want) {
		t.Errorf("DecodeFixed pad: got %x, want %x", got, want)
	}
}

func TestEncode_LeadingZeros(t *testing.T) {
	b := []byte{0x00, 0x00, 0x01}
	enc := Encode(b)
	// Two leading zero bytes → two leading '1' characters.
	if len(enc) < 2 || enc[0] != '1' || enc[1] != '1' {
		t.Errorf("Encode leading zeros: got %q, expected at least two leading '1'", enc)
	}
	dec, err := Decode(enc)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(dec, b) {
		t.Errorf("round-trip leading zeros: got %x, want %x", dec, b)
	}
}
