package users

import "golang.org/x/crypto/bcrypt"

// Convert string password to bytes
// using  "golang.org/x/crypto/bcrypt" package
func hashPassword(password string) ([]byte, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return bytes, err
}

// Verfy if encrypted password and raw password are equal
func checkPasswordHash(password string, hash []byte) bool {
	err := bcrypt.CompareHashAndPassword(hash, []byte(password))
	return err == nil
}
