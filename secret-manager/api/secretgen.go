// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"crypto/rand"
	"math/big"
)

const (
	SecretLength = 32
	SecretPrefix = "trd_"

	upperLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lowerLetters = "abcdefghijklmnopqrstuvwxyz"
	digits       = "0123456789"
	specialChars = "!@#%^&*()-_=+[]{}|;:,.?"
	allChars     = upperLetters + lowerLetters + digits + specialChars
)

// GenerateSecret generates a random secret of the default length with the default prefix.
// It will contain uppercase letters, lowercase letters, digits, and special characters to ensure complexity.
func GenerateSecret() string {
	return GenerateSecretWithOptions(SecretPrefix, SecretLength)
}

// GenerateSecGenerateSecretWithOptions ret generates a random secret of the specified length with the defined prefix.
// It will contain uppercase letters, lowercase letters, digits, and special characters to ensure complexity.
func GenerateSecretWithOptions(prefix string, length int) string {
	if length <= len(prefix) {
		return prefix
	}

	secretLen := length - len(prefix)

	// Ensure at least one character from each category
	secret := make([]byte, secretLen)
	required := []string{upperLetters, lowerLetters, digits, specialChars}
	for i, charset := range required {
		if i >= secretLen {
			break
		}
		secret[i] = randChar(charset)
	}

	// Fill the rest with random characters from all categories
	for i := len(required); i < secretLen; i++ {
		secret[i] = randChar(allChars)
	}

	// Shuffle to avoid predictable positions for required characters
	shuffle(secret)

	return prefix + string(secret)
}

func randChar(charset string) byte {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
	return charset[n.Int64()]
}

func shuffle(b []byte) {
	for i := len(b) - 1; i > 0; i-- {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := n.Int64()
		b[i], b[j] = b[j], b[i]
	}
}
