package utils

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Characters that are unsafe for filenames
	unsafeChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	// Multiple spaces or dots
	multiSpaces = regexp.MustCompile(`\s+`)
	multiDots   = regexp.MustCompile(`\.+`)
)

// SanitizeFilename removes or replaces characters that are unsafe for filenames
func SanitizeFilename(name string) string {
	// Replace unsafe characters with underscore
	sanitized := unsafeChars.ReplaceAllString(name, "_")

	// Replace multiple spaces with single space
	sanitized = multiSpaces.ReplaceAllString(sanitized, " ")

	// Replace multiple dots with single dot
	sanitized = multiDots.ReplaceAllString(sanitized, ".")

	// Trim spaces, dots, and underscores from start and end
	sanitized = strings.Trim(sanitized, " ._")

	// Ensure filename is not empty or only underscores
	if sanitized == "" || strings.Trim(sanitized, "_") == "" {
		sanitized = "untitled"
	}

	// Limit length to 255 characters (common filesystem limit)
	if len(sanitized) > 255 {
		sanitized = sanitized[:255]
	}

	return sanitized
}

// BuildOutputPath constructs the output path from fragments and title
// output/<frag1>/<frag2>/<frag3>/<frag4>/<frag5>/<title>.md
func BuildOutputPath(baseDir, title string, fragments []string) string {
	// Filter out empty fragments
	var parts []string
	for _, frag := range fragments {
		if frag != "" {
			parts = append(parts, SanitizeFilename(frag))
		}
	}

	// Add sanitized title with .md extension
	filename := SanitizeFilename(title) + ".md"
	parts = append(parts, filename)

	// Join all parts
	return filepath.Join(append([]string{baseDir}, parts...)...)
}

// CalculateRelativePath calculates the relative path from source to target
// based on their fragment hierarchies
func CalculateRelativePath(sourceFragments, targetFragments []string, targetTitle string) string {
	// Filter empty fragments
	var srcParts, tgtParts []string
	for _, frag := range sourceFragments {
		if frag != "" {
			srcParts = append(srcParts, SanitizeFilename(frag))
		}
	}
	for _, frag := range targetFragments {
		if frag != "" {
			tgtParts = append(tgtParts, SanitizeFilename(frag))
		}
	}

	// Add target filename
	tgtParts = append(tgtParts, SanitizeFilename(targetTitle)+".md")

	// Find common prefix
	commonLen := 0
	minLen := len(srcParts)
	if len(tgtParts)-1 < minLen { // -1 because target has filename
		minLen = len(tgtParts) - 1
	}

	for i := 0; i < minLen; i++ {
		if srcParts[i] == tgtParts[i] {
			commonLen++
		} else {
			break
		}
	}

	// Build relative path
	// Go up from source depth
	upLevels := len(srcParts) - commonLen
	var relParts []string
	for i := 0; i < upLevels; i++ {
		relParts = append(relParts, "..")
	}

	// Add remaining target parts
	relParts = append(relParts, tgtParts[commonLen:]...)

	return filepath.Join(relParts...)
}

// EnsureUniquePath ensures the path is unique by appending a number if necessary
func EnsureUniquePath(path string, existingPaths map[string]bool) string {
	if !existingPaths[path] {
		return path
	}

	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)

	for i := 1; ; i++ {
		newPath := fmt.Sprintf("%s_%d%s", base, i, ext)
		if !existingPaths[newPath] {
			return newPath
		}
	}
}

// NormalizeFilename normalizes a filename to be lowercase, hyphenated, and without special characters
func NormalizeFilename(filename string) string {
	// Strip file extension if present
	if idx := strings.LastIndex(filename, "."); idx != -1 {
		filename = filename[:idx]
	}

	// Convert to lowercase
	filename = strings.ToLower(filename)

	// Replace spaces with hyphens
	filename = strings.ReplaceAll(filename, " ", "-")

	// Remove special characters, keeping only alphanumeric and hyphens
	var sb strings.Builder
	for _, r := range filename {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			sb.WriteRune(r)
		}
	}
	filename = sb.String()

	// Replace multiple consecutive hyphens with a single hyphen
	for strings.Contains(filename, "--") {
		filename = strings.ReplaceAll(filename, "--", "-")
	}

	// Trim hyphens from start and end
	filename = strings.Trim(filename, "-")

	// If filename is empty after normalization, use a default
	if filename == "" {
		filename = "unnamed"
	}

	return filename
}
