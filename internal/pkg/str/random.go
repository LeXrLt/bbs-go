package str

import (
	"crypto/rand"
	"math/big"
)

const (
	// Character set for password generation
	charset        = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	passwordLength = 20
)

// GenerateRandomPassword creates a cryptographically secure random password.
func GenerateRandomPassword() (string, error) {
	b := make([]byte, passwordLength)
	for i := range b {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[index.Int64()]
	}
	return string(b), nil
}
