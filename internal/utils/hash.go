package utils

import (
	"crypto/sha256"
	"fmt"
)

// CalculateContentHash generates a SHA256 hash of the content
func CalculateContentHash(content []byte) string {
	hash := sha256.Sum256(content)
	return fmt.Sprintf("%x", hash)
}

// CalculateStringHash generates a SHA256 hash of a string
func CalculateStringHash(content string) string {
	return CalculateContentHash([]byte(content))
}
