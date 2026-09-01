package services

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPIN securely hashes a PIN using bcrypt.
func HashPIN(pin string) (string, error) {
	if len(pin) < 1 {
		return "", fmt.Errorf("PIN cannot be empty")
	}
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash PIN: %w", err)
	}
	return string(hashedBytes), nil
}

// VerifyPIN checks a plain PIN against a bcrypt hash.
func VerifyPIN(pin, hash string) bool {
	if pin == "" || hash == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(pin))
	return err == nil
}
