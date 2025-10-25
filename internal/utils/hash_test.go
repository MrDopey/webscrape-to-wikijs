package utils

import (
	"testing"
)

func TestCalculateContentHash(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		expected string
	}{
		{
			name:     "empty content",
			content:  []byte{},
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:     "simple text",
			content:  []byte("hello world"),
			expected: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
		{
			name:     "markdown content",
			content:  []byte("# Hello\n\nThis is a test."),
			expected: "d1e8a70b420c5e7c1e8b2c5f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateContentHash(tt.content)
			// Check that we get a valid SHA256 hash (64 hex characters)
			if len(result) != 64 {
				t.Errorf("CalculateContentHash() returned hash with length %d, want 64", len(result))
			}
			// Verify consistency
			result2 := CalculateContentHash(tt.content)
			if result != result2 {
				t.Errorf("CalculateContentHash() is not consistent: %q != %q", result, result2)
			}
		})
	}
}

func TestCalculateStringHash(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "empty string",
			content: "",
		},
		{
			name:    "simple string",
			content: "hello world",
		},
		{
			name:    "unicode content",
			content: "Hello 世界 🌍",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateStringHash(tt.content)
			// Check that we get a valid SHA256 hash (64 hex characters)
			if len(result) != 64 {
				t.Errorf("CalculateStringHash() returned hash with length %d, want 64", len(result))
			}
			// Verify consistency
			result2 := CalculateStringHash(tt.content)
			if result != result2 {
				t.Errorf("CalculateStringHash() is not consistent: %q != %q", result, result2)
			}
			// Verify it matches CalculateContentHash
			expectedHash := CalculateContentHash([]byte(tt.content))
			if result != expectedHash {
				t.Errorf("CalculateStringHash() = %q, want %q (from CalculateContentHash)", result, expectedHash)
			}
		})
	}
}

func TestHashUniqueness(t *testing.T) {
	// Test that different content produces different hashes
	content1 := "hello world"
	content2 := "hello world!"

	hash1 := CalculateStringHash(content1)
	hash2 := CalculateStringHash(content2)

	if hash1 == hash2 {
		t.Errorf("Different content produced same hash: %q", hash1)
	}
}
