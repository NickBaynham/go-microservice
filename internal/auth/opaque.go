package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// NewRefreshTokenPair returns a high-entropy plaintext token for the client and a SHA-256 hex digest for storage.
func NewRefreshTokenPair() (plaintext string, hashHex string, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", fmt.Errorf("refresh token entropy: %w", err)
	}
	plaintext = hex.EncodeToString(b[:])
	sum := sha256.Sum256([]byte(plaintext))
	hashHex = hex.EncodeToString(sum[:])
	return plaintext, hashHex, nil
}

// HashRefreshToken returns the SHA-256 hex digest of the plaintext refresh token.
func HashRefreshToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
