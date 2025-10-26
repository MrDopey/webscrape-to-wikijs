package csv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseInputCSV(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		csvContent  string
		expectedLen int
		expectError bool
	}{
		{
			name: "valid CSV with url column",
			csvContent: `url
https://drive.google.com/file/d/1AbCdEf/view
https://drive.google.com/file/d/2BcDeF/view`,
			expectedLen: 2,
			expectError: false,
		},
		{
			name: "valid CSV with link column",
			csvContent: `link
https://drive.google.com/file/d/1AbCdEf/view
https://drive.google.com/file/d/2BcDeF/view`,
			expectedLen: 2,
			expectError: false,
		},
		{
			name: "CSV with empty lines",
			csvContent: `url
https://drive.google.com/file/d/1AbCdEf/view

https://drive.google.com/file/d/2BcDeF/view`,
			expectedLen: 2,
			expectError: false,
		},
		{
			name: "CSV with extra columns",
			csvContent: `url,title,extra
https://drive.google.com/file/d/1AbCdEf/view,Title 1,Extra 1
https://drive.google.com/file/d/2BcDeF/view,Title 2,Extra 2`,
			expectedLen: 2,
			expectError: false,
		},
		{
			name: "no url or link column",
			csvContent: `name,value
Item 1,Value 1
Item 2,Value 2`,
			expectedLen: 0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp CSV file
			csvPath := filepath.Join(tempDir, "test.csv")
			err := os.WriteFile(csvPath, []byte(tt.csvContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test CSV: %v", err)
			}

			// Parse CSV
			records, err := ParseInputCSV(csvPath)

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

			if len(records) != tt.expectedLen {
				t.Errorf("Got %d records, want %d", len(records), tt.expectedLen)
			}

			// Verify records have URLs
			for i, record := range records {
				if record.URL == "" {
					t.Errorf("Record %d has empty URL", i)
				}
			}
		})
	}
}

func TestParseConversionCSV(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		csvContent  string
		expectedLen int
		expectError bool
	}{
		{
			name: "valid conversion CSV",
			csvContent: `link,title,tags,frag1,frag2,frag3,frag4,frag5
https://drive.google.com/file/d/1AbCdEf/view,Doc 1,tag1;tag2,guides,tutorials,,,
https://drive.google.com/file/d/2BcDeF/view,Doc 2,tag3,reference,api,,,`,
			expectedLen: 2,
			expectError: false,
		},
		{
			name: "CSV with empty optional fields",
			csvContent: `link,title,tags,frag1,frag2,frag3,frag4,frag5
https://drive.google.com/file/d/1AbCdEf/view,Doc 1,,,,,,
https://drive.google.com/file/d/2BcDeF/view,Doc 2,,guides,,,,`,
			expectedLen: 2,
			expectError: false,
		},
		{
			name: "missing required column",
			csvContent: `link,tags,frag1
https://drive.google.com/file/d/1AbCdEf/view,tag1,guides`,
			expectedLen: 0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp CSV file
			csvPath := filepath.Join(tempDir, "test.csv")
			err := os.WriteFile(csvPath, []byte(tt.csvContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test CSV: %v", err)
			}

			// Parse CSV
			records, err := ParseConversionCSV(csvPath)

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

			if len(records) != tt.expectedLen {
				t.Errorf("Got %d records, want %d", len(records), tt.expectedLen)
			}

			// Verify records have required fields
			for i, record := range records {
				if record.Link == "" {
					t.Errorf("Record %d has empty Link", i)
				}
				if record.Title == "" {
					t.Errorf("Record %d has empty Title", i)
				}
			}
		})
	}
}

func TestConversionRecordGetFragments(t *testing.T) {
	record := ConversionRecord{
		Frag1: "guides",
		Frag2: "tutorials",
		Frag3: "",
		Frag4: "",
		Frag5: "",
	}

	fragments := record.GetFragments()

	if len(fragments) != 5 {
		t.Errorf("GetFragments() returned %d fragments, want 5", len(fragments))
	}

	if fragments[0] != "guides" || fragments[1] != "tutorials" {
		t.Errorf("GetFragments() = %v, want [guides tutorials   ]", fragments)
	}
}

func TestConversionRecordGetTagsList(t *testing.T) {
	tests := []struct {
		name     string
		tags     string
		expected []string
	}{
		{
			name:     "single tag",
			tags:     "tutorial",
			expected: []string{"tutorial"},
		},
		{
			name:     "multiple tags with semicolon",
			tags:     "tutorial;beginner;guide",
			expected: []string{"tutorial", "beginner", "guide"},
		},
		{
			name:     "tags with extra spaces",
			tags:     "tutorial;  beginner;   guide",
			expected: []string{"tutorial", "beginner", "guide"},
		},
		{
			name:     "empty tags",
			tags:     "",
			expected: nil,
		},
		{
			name:     "comma in tags (not a separator)",
			tags:     "tutorial, advanced",
			expected: []string{"tutorial, advanced"}, // Comma is part of the tag, not a separator
		},
		{
			name:     "mixed whitespace",
			tags:     "  tutorial  ;  beginner  ;  guide  ",
			expected: []string{"tutorial", "beginner", "guide"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := ConversionRecord{Tags: tt.tags}
			result := record.GetTagsList()

			if len(result) != len(tt.expected) {
				t.Errorf("GetTagsList() returned %d tags, want %d", len(result), len(tt.expected))
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("GetTagsList()[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}
