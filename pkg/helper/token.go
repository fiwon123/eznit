package helper

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// Use to generate a json using crypto/rand package
func GenerateToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// Fingerprint returns a short, non-reversible identifier for a token.
//
// A session token IS the bearer credential: whoever holds it can act as that
// user. Writing the raw token to a log file therefore hands the credential to
// everyone who can read that file -- and the log file is persisted, rotated
// and kept for 30 days, which stdout is not.
//
// The fingerprint is enough to follow one session through the logs ("this
// request, that request, and the logout all carry token_fp a1b2c3d4") and
// useless to anyone who steals it: it cannot be replayed as a credential.
func Fingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:8]
}
