package helper

import (
	"crypto/rand"
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
