// Package session provides AES-GCM cookie encryption and login rate limiting.
package session

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
)

const errSessionKeyNotConfigured = "session encryption key is not configured"

func EncryptJSONValue(key []byte, data any) (string, error) {
	if len(key) != 32 {
		return "", errors.New(errSessionKeyNotConfigured)
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, payload, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func DecryptJSONValue(key []byte, value string, out any) error {
	if len(key) != 32 {
		return errors.New(errSessionKeyNotConfigured)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if len(decoded) < gcm.NonceSize() {
		return errors.New("invalid session payload")
	}
	nonce := decoded[:gcm.NonceSize()]
	ciphertext := decoded[gcm.NonceSize():]
	payload, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}
