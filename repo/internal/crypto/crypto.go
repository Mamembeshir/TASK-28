// Package crypto provides AES-256-GCM authenticated encryption helpers used
// for sensitive-field-at-rest compliance across the application.
//
// # Password hash encryption
//
// bcrypt hashes are computationally expensive to brute-force, but storing
// them as plaintext in the database exposes them to SQL-injection and DB-dump
// attacks.  This package wraps the raw bcrypt hash with AES-256-GCM before
// writing it to the password_hash column, so that the encryption key (never
// stored in the DB) must be known to recover the hash.
//
// Stored format: hex(nonce || ciphertext) where ciphertext = GCM.Seal(hash)
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

// Encrypt encrypts plaintext using AES-256-GCM with the supplied 32-byte key
// and returns a hex-encoded string (nonce‖ciphertext).
func Encrypt(key []byte, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: read nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return hex.EncodeToString(ciphertext), nil
}

// Decrypt reverses Encrypt.  Returns the original plaintext or an error if
// the key is wrong or the data is corrupted.
func Decrypt(key []byte, encoded string) ([]byte, error) {
	data, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("crypto: hex decode: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, fmt.Errorf("crypto: ciphertext too short")
	}
	nonce, ciphertext := data[:ns], data[ns:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}
