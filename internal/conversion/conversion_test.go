package conversion

import (
	"testing"

	"github.com/yourusername/webscrape-to-wikijs/internal/utils"
)

func TestExtractFileID(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "Google Docs URL",
			url:  "https://docs.google.com/document/d/abc123def456/edit",
			want: "abc123def456",
		},
		{
			name: "Google Sheets URL",
			url:  "https://docs.google.com/spreadsheets/d/xyz789uvw012/edit",
			want: "xyz789uvw012",
		},
		{
			name: "Google Slides URL",
			url:  "https://docs.google.com/presentation/d/pqr345stu678/edit",
			want: "pqr345stu678",
		},
		{
			name: "Google Forms URL with /d/e/ pattern",
			url:  "https://docs.google.com/forms/d/e/1FAIpQLSd_example_form_id/viewform",
			want: "1FAIpQLSd_example_form_id",
		},
		{
			name: "Google Drive file URL",
			url:  "https://drive.google.com/file/d/file123456/view",
			want: "file123456",
		},
		{
			name: "Google Drive folder URL - can extract ID",
			url:  "https://drive.google.com/drive/folders/folder789012",
			want: "folder789012", // ID can be extracted even though folders aren't converted
		},
		{
			name:    "Invalid URL - not Google Drive",
			url:     "https://example.com/document/123",
			wantErr: true,
		},
		{
			name:    "Invalid URL - malformed",
			url:     "not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := utils.ExtractFileID(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractFileID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ExtractFileID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequiresStubConversion(t *testing.T) {
	c := &Converter{}

	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "Google Form URL",
			url:  "https://docs.google.com/forms/d/e/123/viewform",
			want: true,
		},
		{
			name: "Google Sheets URL",
			url:  "https://docs.google.com/spreadsheets/d/abc123/edit",
			want: true,
		},
		{
			name: "Google Presentation URL",
			url:  "https://docs.google.com/presentation/d/xyz789/edit",
			want: true,
		},
		{
			name: "Google Docs URL - should not require stub",
			url:  "https://docs.google.com/document/d/abc123/edit",
			want: false,
		},
		{
			name: "Google Drive file URL - should not require stub",
			url:  "https://drive.google.com/file/d/abc123/view",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.requiresStubConversion(tt.url); got != tt.want {
				t.Errorf("requiresStubConversion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsUnsupportedMediaType(t *testing.T) {
	c := &Converter{}

	tests := []struct {
		name     string
		mimeType string
		want     bool
	}{
		{
			name:     "Video MP4",
			mimeType: "video/mp4",
			want:     true,
		},
		{
			name:     "Video QuickTime",
			mimeType: "video/quicktime",
			want:     true,
		},
		{
			name:     "Audio MP3",
			mimeType: "audio/mpeg",
			want:     true,
		},
		{
			name:     "Audio WAV",
			mimeType: "audio/wav",
			want:     true,
		},
		{
			name:     "Image JPEG",
			mimeType: "image/jpeg",
			want:     true,
		},
		{
			name:     "Image PNG",
			mimeType: "image/png",
			want:     true,
		},
		{
			name:     "Google Presentation",
			mimeType: "application/vnd.google-apps.presentation",
			want:     true,
		},
		{
			name:     "Google Spreadsheet",
			mimeType: "application/vnd.google-apps.spreadsheet",
			want:     true,
		},
		{
			name:     "PowerPoint",
			mimeType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
			want:     true,
		},
		{
			name:     "Excel",
			mimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			want:     true,
		},
		{
			name:     "Google Docs - should be supported",
			mimeType: "application/vnd.google-apps.document",
			want:     false,
		},
		{
			name:     "PDF - should be supported",
			mimeType: "application/pdf",
			want:     false,
		},
		{
			name:     "Word Document - should be supported",
			mimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.isUnsupportedMediaType(tt.mimeType); got != tt.want {
				t.Errorf("isUnsupportedMediaType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDocumentType(t *testing.T) {
	c := &Converter{}

	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "Google Form",
			url:  "https://docs.google.com/forms/d/e/123/viewform",
			want: "Google Form",
		},
		{
			name: "Google Sheet",
			url:  "https://docs.google.com/spreadsheets/d/abc123/edit",
			want: "Google Sheet",
		},
		{
			name: "Google Presentation",
			url:  "https://docs.google.com/presentation/d/xyz789/edit",
			want: "Google Presentation",
		},
		{
			name: "Other Google Document",
			url:  "https://docs.google.com/document/d/abc123/edit",
			want: "Google Document",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.getDocumentType(tt.url); got != tt.want {
				t.Errorf("getDocumentType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDocumentTypeFromMimeType(t *testing.T) {
	c := &Converter{}

	tests := []struct {
		name     string
		mimeType string
		want     string
	}{
		{
			name:     "Video file",
			mimeType: "video/mp4",
			want:     "video file",
		},
		{
			name:     "Audio file",
			mimeType: "audio/mpeg",
			want:     "audio file",
		},
		{
			name:     "Image file",
			mimeType: "image/jpeg",
			want:     "image file",
		},
		{
			name:     "Google Presentation",
			mimeType: "application/vnd.google-apps.presentation",
			want:     "Google Presentation",
		},
		{
			name:     "Google Sheet",
			mimeType: "application/vnd.google-apps.spreadsheet",
			want:     "Google Sheet",
		},
		{
			name:     "PowerPoint",
			mimeType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
			want:     "PowerPoint presentation",
		},
		{
			name:     "Excel",
			mimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			want:     "Excel spreadsheet",
		},
		{
			name:     "Unknown media type",
			mimeType: "application/octet-stream",
			want:     "media file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.getDocumentTypeFromMimeType(tt.mimeType); got != tt.want {
				t.Errorf("getDocumentTypeFromMimeType() = %v, want %v", got, tt.want)
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
			name:     "Simple filename",
			filename: "Getting Started",
			want:     "getting-started",
		},
		{
			name:     "Filename with special characters",
			filename: "API Reference: v2.0!",
			want:     "api-reference-v2", // Note: .0 is treated as extension and removed
		},
		{
			name:     "Filename with multiple spaces",
			filename: "User   Guide    2024",
			want:     "user-guide-2024",
		},
		{
			name:     "Filename with underscores",
			filename: "test_file_name",
			want:     "testfilename", // Note: underscores are removed, not converted to hyphens
		},
		{
			name:     "Filename with extension",
			filename: "document.md",
			want:     "document",
		},
		{
			name:     "Filename with leading/trailing hyphens",
			filename: "--test--",
			want:     "test",
		},
		{
			name:     "Empty after normalization",
			filename: "!@#$%",
			want:     "unnamed",
		},
		{
			name:     "Uppercase with numbers",
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
