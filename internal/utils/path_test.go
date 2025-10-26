package utils

import (
	"path/filepath"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean filename",
			input:    "normal-file.md",
			expected: "normal-file.md",
		},
		{
			name:     "special characters",
			input:    "file:with<bad>chars",
			expected: "file_with_bad_chars",
		},
		{
			name:     "multiple spaces",
			input:    "file   with   spaces",
			expected: "file with spaces",
		},
		{
			name:     "leading and trailing spaces",
			input:    "  file  ",
			expected: "file",
		},
		{
			name:     "multiple dots",
			input:    "file...with...dots",
			expected: "file.with.dots",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "untitled",
		},
		{
			name:     "only special chars",
			input:    "<<<>>>",
			expected: "untitled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBuildOutputPath(t *testing.T) {
	tests := []struct {
		name      string
		baseDir   string
		title     string
		fragments []string
		expected  string
	}{
		{
			name:      "all fragments",
			baseDir:   "/output",
			title:     "Test Document",
			fragments: []string{"guides", "tutorials", "basic", "", ""},
			expected:  filepath.Join("/output", "guides", "tutorials", "basic", "Test Document.md"),
		},
		{
			name:      "no fragments",
			baseDir:   "/output",
			title:     "Test Document",
			fragments: []string{"", "", "", "", ""},
			expected:  filepath.Join("/output", "Test Document.md"),
		},
		{
			name:      "some fragments",
			baseDir:   "/output",
			title:     "API Reference",
			fragments: []string{"docs", "api", "", "", ""},
			expected:  filepath.Join("/output", "docs", "api", "API Reference.md"),
		},
		{
			name:      "fragments with special chars",
			baseDir:   "/output",
			title:     "Test:Doc",
			fragments: []string{"guides/bad", "test<>", "", "", ""},
			expected:  filepath.Join("/output", "guides_bad", "test", "Test_Doc.md"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildOutputPath(tt.baseDir, tt.title, tt.fragments)
			if result != tt.expected {
				t.Errorf("BuildOutputPath() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestCalculateRelativePath(t *testing.T) {
	tests := []struct {
		name            string
		sourceFragments []string
		targetFragments []string
		targetTitle     string
		expected        string
	}{
		{
			name:            "same directory",
			sourceFragments: []string{"guides", "tutorials", "", "", ""},
			targetFragments: []string{"guides", "tutorials", "", "", ""},
			targetTitle:     "target",
			expected:        "target.md",
		},
		{
			name:            "sibling directory",
			sourceFragments: []string{"guides", "tutorials", "", "", ""},
			targetFragments: []string{"guides", "reference", "", "", ""},
			targetTitle:     "target",
			expected:        filepath.Join("..", "reference", "target.md"),
		},
		{
			name:            "parent to child",
			sourceFragments: []string{"guides", "", "", "", ""},
			targetFragments: []string{"guides", "tutorials", "basic", "", ""},
			targetTitle:     "target",
			expected:        filepath.Join("tutorials", "basic", "target.md"),
		},
		{
			name:            "child to parent",
			sourceFragments: []string{"guides", "tutorials", "basic", "", ""},
			targetFragments: []string{"guides", "", "", "", ""},
			targetTitle:     "target",
			expected:        filepath.Join("..", "..", "target.md"),
		},
		{
			name:            "different top level",
			sourceFragments: []string{"guides", "tutorials", "", "", ""},
			targetFragments: []string{"reference", "api", "", "", ""},
			targetTitle:     "target",
			expected:        filepath.Join("..", "..", "reference", "api", "target.md"),
		},
		{
			name:            "empty source",
			sourceFragments: []string{"", "", "", "", ""},
			targetFragments: []string{"guides", "tutorials", "", "", ""},
			targetTitle:     "target",
			expected:        filepath.Join("guides", "tutorials", "target.md"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateRelativePath(tt.sourceFragments, tt.targetFragments, tt.targetTitle)
			if result != tt.expected {
				t.Errorf("CalculateRelativePath() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestEnsureUniquePath(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		existingPaths map[string]bool
		expected      string
	}{
		{
			name:          "unique path",
			path:          "/output/file.md",
			existingPaths: map[string]bool{},
			expected:      "/output/file.md",
		},
		{
			name: "duplicate path",
			path: "/output/file.md",
			existingPaths: map[string]bool{
				"/output/file.md": true,
			},
			expected: "/output/file_1.md",
		},
		{
			name: "multiple duplicates",
			path: "/output/file.md",
			existingPaths: map[string]bool{
				"/output/file.md":   true,
				"/output/file_1.md": true,
				"/output/file_2.md": true,
			},
			expected: "/output/file_3.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnsureUniquePath(tt.path, tt.existingPaths)
			if result != tt.expected {
				t.Errorf("EnsureUniquePath() = %q, want %q", result, tt.expected)
			}
		})
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
			if got := NormalizeFilename(tt.filename); got != tt.want {
				t.Errorf("NormalizeFilename() = %v, want %v", got, tt.want)
			}
		})
	}
}
