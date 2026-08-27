package session

import (
	"bytes"
	"testing"
)

func TestEncryptJSONValue_RoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	type payload struct {
		JWT string `json:"jwt"`
	}
	want := payload{JWT: "Bearer token-value"}

	encrypted, err := EncryptJSONValue(key, want)
	if err != nil {
		t.Fatalf("EncryptJSONValue returned error: %v", err)
	}
	if encrypted == "" {
		t.Fatal("EncryptJSONValue returned an empty payload")
	}

	var got payload
	if err := DecryptJSONValue(key, encrypted, &got); err != nil {
		t.Fatalf("DecryptJSONValue returned error: %v", err)
	}
	if got != want {
		t.Fatalf("DecryptJSONValue returned %+v, want %+v", got, want)
	}
}

func TestEncryptJSONValue_InvalidKeyLength(t *testing.T) {
	if _, err := EncryptJSONValue([]byte("short"), map[string]string{"jwt": "token"}); err == nil {
		t.Fatal("EncryptJSONValue should reject keys that are not 32 bytes")
	}
}

func TestDecryptJSONValue_InvalidPayload(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	var got map[string]string

	if err := DecryptJSONValue(key, "not-base64", &got); err == nil {
		t.Fatal("DecryptJSONValue should reject invalid payloads")
	}
}

func TestDecryptJSONValue_TamperedPayload(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	encrypted, err := EncryptJSONValue(key, map[string]string{"jwt": "token"})
	if err != nil {
		t.Fatalf("EncryptJSONValue returned error: %v", err)
	}

	// Corrupt a character in the interior so all six base64url bits carry meaning.
	// The last 1–2 chars of a RawURL string can have padding bits that the decoder
	// ignores, making last-character substitutions non-deterministic.
	mid := len(encrypted) / 2
	orig := encrypted[mid]
	sub := byte('A')
	if orig == 'A' {
		sub = 'B'
	}
	tampered := encrypted[:mid] + string(sub) + encrypted[mid+1:]
	var got map[string]string
	if err := DecryptJSONValue(key, tampered, &got); err == nil {
		t.Fatal("DecryptJSONValue should reject tampered payloads")
	}
}
