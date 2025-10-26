package utils

import (
	"testing"
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
			name: "Google Drive folder URL",
			url:  "https://drive.google.com/drive/folders/folder789012",
			want: "folder789012",
		},
		{
			name: "URL with query parameters",
			url:  "https://docs.google.com/document/d/abc123/edit?usp=sharing",
			want: "abc123",
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
		{
			name:    "Empty URL",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractFileID(tt.url)
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

func TestBuildFileLink(t *testing.T) {
	tests := []struct {
		name     string
		fileID   string
		mimeType string
		want     string
	}{
		{
			name:     "Google Docs",
			fileID:   "abc123",
			mimeType: "application/vnd.google-apps.document",
			want:     "https://docs.google.com/document/d/abc123/edit",
		},
		{
			name:     "Google Sheets",
			fileID:   "xyz789",
			mimeType: "application/vnd.google-apps.spreadsheet",
			want:     "https://docs.google.com/spreadsheets/d/xyz789/edit",
		},
		{
			name:     "Google Slides",
			fileID:   "pqr456",
			mimeType: "application/vnd.google-apps.presentation",
			want:     "https://docs.google.com/presentation/d/pqr456/edit",
		},
		{
			name:     "Google Forms",
			fileID:   "form123",
			mimeType: "application/vnd.google-apps.form",
			want:     "https://docs.google.com/forms/d/e/form123/viewform",
		},
		{
			name:     "Google Drawings",
			fileID:   "draw456",
			mimeType: "application/vnd.google-apps.drawing",
			want:     "https://docs.google.com/drawings/d/draw456/edit",
		},
		{
			name:     "Google Drive Folder",
			fileID:   "folder789",
			mimeType: "application/vnd.google-apps.folder",
			want:     "https://drive.google.com/drive/folders/folder789",
		},
		{
			name:     "PDF file",
			fileID:   "pdf123",
			mimeType: "application/pdf",
			want:     "https://drive.google.com/file/d/pdf123/view",
		},
		{
			name:     "Image file",
			fileID:   "img456",
			mimeType: "image/jpeg",
			want:     "https://drive.google.com/file/d/img456/view",
		},
		{
			name:     "Video file",
			fileID:   "vid789",
			mimeType: "video/mp4",
			want:     "https://drive.google.com/file/d/vid789/view",
		},
		{
			name:     "Unknown file type",
			fileID:   "file123",
			mimeType: "application/octet-stream",
			want:     "https://drive.google.com/file/d/file123/view",
		},
		{
			name:     "Empty MIME type",
			fileID:   "file456",
			mimeType: "",
			want:     "https://drive.google.com/file/d/file456/view",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildFileLink(tt.fileID, tt.mimeType); got != tt.want {
				t.Errorf("BuildFileLink() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeMultilineURLs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "Simple URL - no changes needed",
			content: "https://docs.google.com/document/d/abc123/edit",
			want:    "https://docs.google.com/document/d/abc123/edit",
		},
		{
			name:    "URL broken across lines",
			content: "https://docs.google.com/document/d/abc\ndef/edit",
			want:    "https://docs.google.com/document/d/abcdef/edit",
		},
		{
			name:    "URL with escaped underscore",
			content: "https://docs.google.com/document/d/abc\\_def/edit",
			want:    "https://docs.google.com/document/d/abc_def/edit",
		},
		{
			name:    "URL with escaped asterisk",
			content: "https://docs.google.com/document/d/abc\\*def/edit",
			want:    "https://docs.google.com/document/d/abc*def/edit",
		},
		{
			name:    "Multiple URLs",
			content: "https://docs.google.com/document/d/abc123/edit and https://drive.google.com/file/d/xyz789/view",
			want:    "https://docs.google.com/document/d/abc123/edit and https://drive.google.com/file/d/xyz789/view",
		},
		{
			name:    "URL with whitespace around line break",
			content: "https://docs.google.com/document/d/abc  \n  def/edit",
			want:    "https://docs.google.com/document/d/abcdef/edit",
		},
		{
			name:    "Drive URL broken across lines",
			content: "https://drive.google.com/file/d/abc\n123/view",
			want:    "https://drive.google.com/file/d/abc123/view",
		},
		{
			name:    "No URLs",
			content: "This is just plain text with no URLs",
			want:    "This is just plain text with no URLs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeMultilineURLs(tt.content); got != tt.want {
				t.Errorf("NormalizeMultilineURLs() = %v, want %v", got, tt.want)
			}
		})
	}
}
