package utils

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var driveIDPattern = regexp.MustCompile(`[-\w]{25,}`)

// ExtractFileID extracts the file/folder ID from a Google Drive URL
func ExtractFileID(urlStr string) (string, error) {
	// Parse URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Check if it's a Google Drive URL
	if !strings.Contains(u.Host, "drive.google.com") && !strings.Contains(u.Host, "docs.google.com") {
		return "", fmt.Errorf("not a Google Drive URL")
	}

	// Try to extract ID from path
	// Format: /file/d/{id}/...
	// Format: /folders/{id}
	// Format: /document/d/{id}/...
	// Format: /forms/d/e/{id}/... (Google Forms)
	parts := strings.Split(u.Path, "/")
	for i, part := range parts {
		// Check for Google Forms pattern: /forms/d/e/{id}
		if part == "d" && i+1 < len(parts) && parts[i+1] == "e" && i+2 < len(parts) {
			return parts[i+2], nil
		}
		// Standard patterns
		if (part == "d" || part == "folders") && i+1 < len(parts) {
			return parts[i+1], nil
		}
	}

	// Try to extract from query parameter
	if id := u.Query().Get("id"); id != "" {
		return id, nil
	}

	// Try to match ID pattern in the entire URL
	matches := driveIDPattern.FindStringSubmatch(urlStr)
	if len(matches) > 0 {
		return matches[0], nil
	}

	return "", fmt.Errorf("could not extract file ID from URL: %s", urlStr)
}

// NormalizeMultilineURLs fixes Google Drive/Docs URLs that are broken across multiple lines
// and unescapes markdown characters within URLs
// Example: "*https://docs.google.com/document/d/abc*\n*defg/edit*" -> "https://docs.google.com/document/d/abcdefg/edit"
// Example: "https://docs.google.com/document/d/abc\_def/edit" -> "https://docs.google.com/document/d/abc_def/edit"
func NormalizeMultilineURLs(content string) string {
	// Step 1: Fix URLs broken across adjacent lines FIRST (before unescaping)
	// Pattern to match Google Drive URL that might continue on next line
	// Excludes * and _ from URL parts so they're only matched as markdown delimiters
	// Explicitly matches and strips markdown markers (*/_) around the line break
	// Only joins adjacent pairs - does not handle URLs broken across 3+ lines
	urlContinuationPattern := regexp.MustCompile(
		`(https://(?:drive\.google\.com|docs\.google\.com)/[^\s\n*_]+)[\*_]*\s*\n\s*[\*_]*([^\s\n*_]+)[\*_]*`,
	)
	content = urlContinuationPattern.ReplaceAllString(content, "$1$2")

	// Step 2: Unescape markdown characters within Google Drive URLs
	// This handles cases like \_  \*  etc. that are escaped in markdown
	// Do this AFTER joining lines so we unescape the complete URL
	escapedCharsPattern := regexp.MustCompile(`(https://(?:drive\.google\.com|docs\.google\.com)/[^\s\n]*)`)
	content = escapedCharsPattern.ReplaceAllStringFunc(content, func(url string) string {
		// Remove backslash escapes from common markdown characters
		url = strings.ReplaceAll(url, `\_`, `_`)
		url = strings.ReplaceAll(url, `\*`, `*`)
		url = strings.ReplaceAll(url, `\-`, `-`)
		url = strings.ReplaceAll(url, `\[`, `[`)
		url = strings.ReplaceAll(url, `\]`, `]`)
		return url
	})

	return content
}

// BuildFileLink constructs an appropriate Google link from an ID and MIME type
func BuildFileLink(fileID string, mimeType string) string {
	switch mimeType {
	case "application/vnd.google-apps.document":
		return fmt.Sprintf("https://docs.google.com/document/d/%s/edit", fileID)
	case "application/vnd.google-apps.spreadsheet":
		return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit", fileID)
	case "application/vnd.google-apps.presentation":
		return fmt.Sprintf("https://docs.google.com/presentation/d/%s/edit", fileID)
	case "application/vnd.google-apps.form":
		return fmt.Sprintf("https://docs.google.com/forms/d/e/%s/viewform", fileID)
	case "application/vnd.google-apps.drawing":
		return fmt.Sprintf("https://docs.google.com/drawings/d/%s/edit", fileID)
	case "application/vnd.google-apps.folder":
		return fmt.Sprintf("https://drive.google.com/drive/folders/%s", fileID)
	default:
		// For non-Google Workspace files (PDFs, images, etc.)
		return fmt.Sprintf("https://drive.google.com/file/d/%s/view", fileID)
	}
}
