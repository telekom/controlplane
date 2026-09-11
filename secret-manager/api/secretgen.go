// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	SecretLength = 36
	SecretPrefix = "trd_" // 36 - len("trd_") = 32 chars

	upperLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lowerLetters = "abcdefghijklmnopqrstuvwxyz"
	digits       = "0123456789"
	// Special chars might have conflicts with certain backends, so we omit them for maximum compatibility.
	// specialChars = "!@#%^&*()-_=+[]{}|;:,.?"
	allChars = upperLetters + lowerLetters + digits
)

// GenerateSecret generates a random secret of the default length with the default prefix.
// It will contain uppercase letters, lowercase letters, digits, and special characters to ensure complexity.
func GenerateSecret() (string, error) {
	return GenerateSecretWithOptions(SecretPrefix, SecretLength)
}

// GenerateSecretOrDie generates a random secret and panics if there is an error.
func GenerateSecretOrDie() string {
	secret, err := GenerateSecret()
	if err != nil {
		panic(fmt.Sprintf("failed to generate secret: %v", err))
	}
	return secret
}

// GenerateSecretWithOptions generates a random secret of the specified length with the defined prefix.
// It will contain uppercase letters, lowercase letters, digits, and special characters to ensure complexity.
func GenerateSecretWithOptions(prefix string, length int) (string, error) {
	var err error

	if length <= len(prefix) {
		return prefix, fmt.Errorf("secret length must be greater than prefix length")
	}

	secretLen := length - len(prefix)

	// Ensure at least one character from each category
	secret := make([]byte, secretLen)
	required := []string{upperLetters, lowerLetters, digits}
	for i, charset := range required {
		if i >= secretLen {
			break
		}
		max := big.NewInt(int64(len(charset)))
		secret[i], err = randChar(charset, max)
		if err != nil {
			return "", err
		}
	}

	// Fill the rest with random characters from all categories
	allMax := big.NewInt(int64(len(allChars)))
	for i := len(required); i < secretLen; i++ {
		secret[i], err = randChar(allChars, allMax)
		if err != nil {
			return "", err
		}
	}

	// Shuffle to avoid predictable positions for required characters
	if err = shuffle(secret); err != nil {
		return "", err
	}

	return prefix + string(secret), nil
}

func randChar(charset string, max *big.Int) (byte, error) {
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0, fmt.Errorf("crypto/rand failed: %w", err)
	}
	return charset[n.Int64()], nil
}

func shuffle(b []byte) error {
	for i := len(b) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return fmt.Errorf("crypto/rand failed: %w", err)
		}
		j := n.Int64()
		b[i], b[j] = b[j], b[i]
	}
	return nil
}
