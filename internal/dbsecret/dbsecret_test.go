package dbsecret

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
)

func TestCipherRoundTripBindsProjectAndDatabase(t *testing.T) {
	cipher, err := New(bytes.Repeat([]byte{0x42}, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	projectID, databaseID := uuid.New(), uuid.New()
	encoded, err := cipher.Encrypt(projectID, databaseID, []byte("s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := cipher.Decrypt(projectID, databaseID, encoded)
	if err != nil || string(plaintext) != "s3cret" {
		t.Fatalf("Decrypt() = %q, %v", plaintext, err)
	}
	if _, err := cipher.Decrypt(uuid.New(), databaseID, encoded); err == nil {
		t.Fatal("Decrypt accepted a different project")
	}
	if _, err := cipher.Decrypt(projectID, uuid.New(), encoded); err == nil {
		t.Fatal("Decrypt accepted a different database")
	}
	encoded[len(encoded)-1] ^= 1
	if _, err := cipher.Decrypt(projectID, databaseID, encoded); err == nil {
		t.Fatal("Decrypt accepted tampered ciphertext")
	}
}

func TestCipherRejectsShortKeys(t *testing.T) {
	if _, err := New([]byte("short")); err == nil {
		t.Fatal("New accepted a non-32-byte key")
	}
}
