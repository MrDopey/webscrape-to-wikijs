package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	s := &Syncer{}

	tests := []struct {
		name        string
		content     string
		wantFM      map[string]string
		wantBody    string
		expectError bool
	}{
		{
			name: "valid frontmatter with hash-gdrive",
			content: `---
title: Test Document
hash-gdrive: 2024-01-15T10:30:00Z
gdrive-link: https://docs.google.com/document/d/abc123/edit
---
> Link: https://docs.google.com/document/d/abc123/edit

This is the body content.`,
			wantFM: map[string]string{
				"title":       "Test Document",
				"hash-gdrive": "2024-01-15T10:30:00Z",
				"gdrive-link": "https://docs.google.com/document/d/abc123/edit",
			},
			wantBody:    "> Link: https://docs.google.com/document/d/abc123/edit\n\nThis is the body content.",
			expectError: false,
		},
		{
			name: "stub document",
			content: `---
title: Test Form
hash-gdrive: stub
gdrive-link: https://docs.google.com/forms/d/e/abc123/viewform
---
> Link: https://docs.google.com/forms/d/e/abc123/viewform

*This is a Google Form. This document type cannot be exported to markdown format.*`,
			wantFM: map[string]string{
				"title":       "Test Form",
				"hash-gdrive": "stub",
				"gdrive-link": "https://docs.google.com/forms/d/e/abc123/viewform",
			},
			expectError: false,
		},
		{
			name: "frontmatter with special characters in values",
			content: `---
title: "Test: Document with special chars"
tags: "tag1;tag2;tag3"
hash-gdrive: 2024-01-15T10:30:00Z
---

Body content`,
			wantFM: map[string]string{
				"title":       "Test: Document with special chars",
				"tags":        "tag1;tag2;tag3",
				"hash-gdrive": "2024-01-15T10:30:00Z",
			},
			expectError: false,
		},
		{
			name:        "no frontmatter",
			content:     "Just plain markdown content",
			expectError: true,
		},
		{
			name: "unclosed frontmatter",
			content: `---
title: Test
hash-gdrive: 2024-01-15T10:30:00Z

Body without closing marker`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body, err := s.parseFrontmatter(tt.content)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Check frontmatter
			for key, expectedValue := range tt.wantFM {
				if actualValue, exists := fm[key]; !exists {
					t.Errorf("Missing frontmatter key: %s", key)
				} else if actualValue != expectedValue {
					t.Errorf("Frontmatter[%s] = %q, want %q", key, actualValue, expectedValue)
				}
			}

			// Check body if specified
			if tt.wantBody != "" && body != tt.wantBody {
				t.Errorf("Body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestBuildFrontmatter(t *testing.T) {
	s := &Syncer{}

	tests := []struct {
		name     string
		fm       map[string]string
		wantKeys []string
	}{
		{
			name: "standard fields",
			fm: map[string]string{
				"title":        "Test Document",
				"hash-gdrive":  "2024-01-15T10:30:00Z",
				"hash-content": "abc123def456",
				"gdrive-link":  "https://docs.google.com/document/d/abc123/edit",
				"tags":         "tag1;tag2",
			},
			wantKeys: []string{"title", "hash-gdrive", "hash-content", "gdrive-link", "tags"},
		},
		{
			name: "fields with special characters",
			fm: map[string]string{
				"title":       "Test: Document",
				"description": "A description with #hashtag",
				"hash-gdrive": "stub",
			},
			wantKeys: []string{"title", "description", "hash-gdrive"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.buildFrontmatter(tt.fm)

			// Check that result starts and ends with ---
			if result[:4] != "---\n" {
				t.Errorf("Frontmatter doesn't start with ---\\n")
			}
			if result[len(result)-4:] != "---\n" {
				t.Errorf("Frontmatter doesn't end with ---\\n")
			}

			// Check that all expected keys are present
			for _, key := range tt.wantKeys {
				if !contains(result, key+":") {
					t.Errorf("Frontmatter missing key: %s", key)
				}
			}
		})
	}
}

func TestFindMarkdownFiles(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()

	// Create some markdown files
	os.WriteFile(filepath.Join(tempDir, "file1.md"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(tempDir, "file2.md"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(tempDir, "file3.txt"), []byte("content"), 0644)

	// Create subdirectory with markdown
	subDir := filepath.Join(tempDir, "subdir")
	os.Mkdir(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "file4.md"), []byte("content"), 0644)

	s := &Syncer{outputDir: tempDir}

	files, err := s.findMarkdownFiles()
	if err != nil {
		t.Fatalf("findMarkdownFiles() error = %v", err)
	}

	// Should find 3 .md files (file1.md, file2.md, file4.md)
	if len(files) != 3 {
		t.Errorf("findMarkdownFiles() found %d files, want 3", len(files))
	}

	// Check that .txt file was not included
	for _, file := range files {
		if filepath.Ext(file) != ".md" {
			t.Errorf("findMarkdownFiles() included non-markdown file: %s", file)
		}
	}
}

func TestNormalizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{
			name:     "simple filename",
			filename: "Getting Started",
			want:     "getting-started",
		},
		{
			name:     "filename with special characters",
			filename: "API Reference: v2.0!",
			want:     "api-reference-v2",
		},
		{
			name:     "filename with multiple spaces",
			filename: "User   Guide    2024",
			want:     "user-guide-2024",
		},
		{
			name:     "filename with underscores",
			filename: "test_file_name",
			want:     "testfilename",
		},
		{
			name:     "filename with extension",
			filename: "document.md",
			want:     "document",
		},
		{
			name:     "filename with leading/trailing hyphens",
			filename: "--test--",
			want:     "test",
		},
		{
			name:     "empty after normalization",
			filename: "!@#$%",
			want:     "unnamed",
		},
		{
			name:     "uppercase with numbers",
			filename: "Section-5A",
			want:     "section-5a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeFilename(tt.filename); got != tt.want {
				t.Errorf("normalizeFilename() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
