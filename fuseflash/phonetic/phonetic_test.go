package phonetic

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	cases := [][]byte{
		{0x00},
		{0xFF},
		{0xAB, 0xCD},
		make([]byte, 20),  // zero address
		make([]byte, 32),  // zero node ID
	}
	// Set some non-zero bytes for the larger cases.
	for i := range cases[3] {
		cases[3][i] = byte(i)
	}
	for i := range cases[4] {
		cases[4][i] = byte(i * 7)
	}

	for _, input := range cases {
		encoded := EncodeBytes(input)
		decoded, err := DecodeBytes(encoded)
		if err != nil {
			t.Errorf("DecodeBytes(%q): %v", encoded, err)
			continue
		}
		if !bytes.Equal(decoded, input) {
			t.Errorf("round-trip failed: got %v, want %v", decoded, input)
		}
	}
}

func TestDecodeBytesOddWords(t *testing.T) {
	_, err := DecodeBytes("Alpha Bravo Charlie")
	if err == nil {
		t.Error("expected error for odd word count, got nil")
	}
}

func TestDecodeBytesUnknownWord(t *testing.T) {
	_, err := DecodeBytes("Alpha Unknown")
	if err == nil {
		t.Error("expected error for unknown word, got nil")
	}
}

func TestDecodeBytesEmpty(t *testing.T) {
	got, err := DecodeBytes("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestValidateNodeID(t *testing.T) {
	id := make([]byte, 32)
	encoded := EncodeBytes(id)
	if err := ValidateNodeID(encoded); err != nil {
		t.Errorf("ValidateNodeID: %v", err)
	}
}

func TestValidateNodeID_WrongLength(t *testing.T) {
	encoded := EncodeBytes([]byte{0x01, 0x02}) // only 2 bytes
	if err := ValidateNodeID(encoded); err == nil {
		t.Error("expected error for wrong length")
	}
}

func TestValidateAddress(t *testing.T) {
	addr := make([]byte, 20)
	encoded := EncodeBytes(addr)
	if err := ValidateAddress(encoded); err != nil {
		t.Errorf("ValidateAddress: %v", err)
	}
}

func TestNormaliseWords(t *testing.T) {
	got, err := NormaliseWords("ALPHA bravo CHARLIE")
	if err != nil {
		t.Fatalf("NormaliseWords: %v", err)
	}
	want := "Alpha Bravo Charlie"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormaliseWordsEmpty(t *testing.T) {
	_, err := NormaliseWords("")
	if err != ErrEmptyInput {
		t.Errorf("expected ErrEmptyInput, got %v", err)
	}
}

func TestNormaliseWordsUnknown(t *testing.T) {
	// "Zulu" is a valid NATO word but is not included in this 16-word
	// nibble alphabet (which only uses Alpha–Papa, i.e. nibbles 0–15).
	_, err := NormaliseWords("Alpha Zulu")
	if err == nil {
		t.Error("expected error for word not in nibble alphabet ('Zulu')")
	}
}

func TestValidateCaseInsensitive(t *testing.T) {
	// EncodeBytes produces title-case; decoding should be case-insensitive.
	_, err := DecodeBytes("alpha bravo")
	if err != nil {
		t.Errorf("case-insensitive decode: %v", err)
	}
}
