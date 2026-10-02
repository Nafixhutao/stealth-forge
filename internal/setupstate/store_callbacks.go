package setupstate

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

func NewCallbackState(purpose string) (plain string, hash string, expiresAt time.Time, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", time.Time{}, fmt.Errorf("generate %s state: %w", purpose, err)
	}
	plain = base64.RawURLEncoding.EncodeToString(bytes)
	// The raw state is returned only to the redirect URL. Callers should persist
	// the digest, which keeps a database/file dump from becoming a callback
	// credential.
	hashBytes := sha256Bytes([]byte(plain))
	hash = base64.RawURLEncoding.EncodeToString(hashBytes)
	return plain, hash, time.Now().UTC().Add(10 * time.Minute), nil
}

func NewManifestState() (plain string, hash string, expiresAt time.Time, err error) {
	return NewCallbackState("GitHub manifest")
}

func NewOAuthState() (plain string, hash string, expiresAt time.Time, err error) {
	return NewCallbackState("GitHub OAuth")
}

func HashCallbackState(value string) string {
	return base64.RawURLEncoding.EncodeToString(sha256Bytes([]byte(strings.TrimSpace(value))))
}

func hashManifestState(value string) string {
	return HashCallbackState(value)
}

func HashOAuthState(value string) string {
	return HashCallbackState(value)
}
