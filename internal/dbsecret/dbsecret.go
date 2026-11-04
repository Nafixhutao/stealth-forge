// Package dbsecret encrypts managed-database passwords under a dedicated
// operator key. The project and database identity are authenticated so a
// ciphertext cannot be moved to another record.
package dbsecret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
)

const (
	KeySize           = 32
	ciphertextVersion = byte(1)
)

var (
	ErrInvalidKey        = errors.New("managed database secret key must be exactly 32 bytes")
	ErrInvalidCiphertext = errors.New("invalid managed database ciphertext")
)

type Cipher struct {
	aead cipher.AEAD
}

func New(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create managed database AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create managed database AES-GCM cipher: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns version || nonce || authenticated ciphertext.
func (c *Cipher) Encrypt(projectID, databaseID uuid.UUID, plaintext []byte) ([]byte, error) {
	if c == nil || c.aead == nil || projectID == uuid.Nil || databaseID == uuid.Nil {
		return nil, ErrInvalidKey
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate managed database nonce: %w", err)
	}
	result := make([]byte, 1, 1+len(nonce)+len(plaintext)+c.aead.Overhead())
	result[0] = ciphertextVersion
	result = append(result, nonce...)
	result = c.aead.Seal(result, nonce, plaintext, associatedData(projectID, databaseID))
	return result, nil
}

func (c *Cipher) Decrypt(projectID, databaseID uuid.UUID, encoded []byte) ([]byte, error) {
	if c == nil || c.aead == nil || projectID == uuid.Nil || databaseID == uuid.Nil {
		return nil, ErrInvalidKey
	}
	nonceSize := c.aead.NonceSize()
	if len(encoded) < 1+nonceSize+c.aead.Overhead() || encoded[0] != ciphertextVersion {
		return nil, ErrInvalidCiphertext
	}
	nonce := encoded[1 : 1+nonceSize]
	plaintext, err := c.aead.Open(nil, nonce, encoded[1+nonceSize:], associatedData(projectID, databaseID))
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plaintext, nil
}

func associatedData(projectID, databaseID uuid.UUID) []byte {
	return []byte("stealth/managed-database/v1\x00" + projectID.String() + "\x00" + databaseID.String())
}
