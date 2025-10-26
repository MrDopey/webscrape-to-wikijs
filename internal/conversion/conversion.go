package conversion

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ledongthuc/pdf"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"

	"github.com/yourusername/webscrape-to-wikijs/internal/csv"
	"github.com/yourusername/webscrape-to-wikijs/internal/utils"
)

// Converter handles conversion of Google Drive documents to markdown
type Converter struct {
	service       *drive.Service
	outputDir     string
	verbose       bool
	dryRun        bool
	linkMap       map[string]*csv.ConversionRecord // Maps file ID to record
	existingPaths map[string]bool
	mu            sync.Mutex
}

// NewConverter creates a new Converter
func NewConverter(service *drive.Service, outputDir string, verbose, dryRun bool) *Converter {
	return &Converter{
		service:       service,
		outputDir:     outputDir,
		verbose:       verbose,
		dryRun:        dryRun,
		linkMap:       make(map[string]*csv.ConversionRecord),
		existingPaths: make(map[string]bool),
	}
}

// Convert converts all records to markdown files
func (c *Converter) Convert(records []csv.ConversionRecord, workers int) error {
	// Build link map for O(1) lookup - index by both URL and file ID
	for i := range records {
		// Index by the exact URL from CSV
		c.linkMap[records[i].Link] = &records[i]

		// Also index by file ID for cross-format matching
		fileID, err := extractFileID(records[i].Link)
		if err != nil {
			log.Printf("Warning: failed to extract file ID from %s: %v", records[i].Link, err)
			continue
		}
		c.linkMap[fileID] = &records[i]
	}

	// Create worker pool
	jobs := make(chan *csv.ConversionRecord, len(records))
	results := make(chan error, len(records))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for record := range jobs {
				err := c.convertRecord(record)
				results <- err
			}
		}()
	}

	// Send jobs
	for i := range records {
		jobs <- &records[i]
	}
	close(jobs)

	// Wait for completion
	wg.Wait()
	close(results)

	// Check for errors
	var errors []error
	for err := range results {
		if err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		log.Printf("Completed with %d errors", len(errors))
		return fmt.Errorf("conversion had %d errors", len(errors))
	}

	return nil
}

// convertRecord converts a single record
func (c *Converter) convertRecord(record *csv.ConversionRecord) error {
	if c.verbose {
		log.Printf("Converting: %s", record.Title)
	}

	// Extract file ID
	fileID, err := extractFileID(record.Link)
	if err != nil {
		return fmt.Errorf("failed to extract file ID from %s: %w", record.Link, err)
	}

	// Get file metadata
	file, err := c.getFileMetadata(fileID)
	if err != nil {
		return fmt.Errorf("failed to get metadata for %s: %w", fileID, err)
	}

	// Download content based on mime type
	var content []byte
	var revisionHash string

	if strings.HasPrefix(file.MimeType, "application/vnd.google-apps.") {
		// Google Workspace document - export as markdown
		content, revisionHash, err = c.exportAsMarkdown(fileID)
		if err != nil {
			return fmt.Errorf("failed to export %s as markdown: %w", record.Title, err)
		}
	} else if file.MimeType == "application/pdf" {
		// PDF - convert to Google Docs format (like "Open with Google Docs" in UI)
		content, revisionHash, err = c.convertPDFViaGoogleDocs(fileID, file.ModifiedTime)
		if err != nil {
			return fmt.Errorf("failed to convert PDF %s: %w", record.Title, err)
		}
	} else {
		return fmt.Errorf("unsupported file type %s for %s", file.MimeType, record.Title)
	}

	// Rewrite links in content
	contentStr := string(content)
	contentStr = c.rewriteLinks(contentStr, record)

	// Generate frontmatter
	frontmatter := c.generateFrontmatter(record, revisionHash, contentStr)

	// Combine frontmatter and content
	finalContent := frontmatter + "\n" + contentStr

	// Build output path with normalized filename
	normalizedTitle := normalizeFilename(record.Title)
	outputPath := utils.BuildOutputPath(c.outputDir, normalizedTitle, record.GetFragments())

	// Ensure unique path
	c.mu.Lock()
	outputPath = utils.EnsureUniquePath(outputPath, c.existingPaths)
	c.existingPaths[outputPath] = true
	c.mu.Unlock()

	if c.dryRun {
		log.Printf("Would write: %s", outputPath)
		return nil
	}

	// Create directory structure
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write file
	if err := os.WriteFile(outputPath, []byte(finalContent), 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", outputPath, err)
	}

	if c.verbose {
		log.Printf("Wrote: %s", outputPath)
	}

	return nil
}

// exportAsMarkdown exports a Google Workspace document as markdown
func (c *Converter) exportAsMarkdown(fileID string) ([]byte, string, error) {
	// Get revision hash
	file, err := c.getFileMetadata(fileID)
	if err != nil {
		return nil, "", err
	}

	// Export as markdown
	body, err := c.executeExportWithRetry(fileID, "text/markdown")
	if err != nil {
		return nil, "", err
	}
	defer body.Close()

	content, err := io.ReadAll(body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %w", err)
	}

	return content, file.ModifiedTime, nil
}

// convertPDFViaGoogleDocs converts a PDF to markdown by creating a Google Docs copy
func (c *Converter) convertPDFViaGoogleDocs(fileID string, modifiedTime string) ([]byte, string, error) {
	if c.verbose {
		log.Printf("Converting PDF %s using Google Docs conversion...", fileID)
	}

	// Create a copy of the PDF as a Google Doc
	// This mimics the "Open with Google Docs" behavior in the UI
	copyFile := &drive.File{
		Parents:  []string{"dev", "temp"},
		Name:     "temp_conversion_" + fileID,
		MimeType: "application/vnd.google-apps.document",
	}

	copiedFile, err := c.service.Files.Copy(fileID, copyFile).SupportsAllDrives(true).Do()
	if err != nil {
		if c.verbose {
			log.Printf("Warning: Failed to convert PDF %s using Google Docs, falling back to text extraction: %v", fileID, err)
		}
		// Fall back to direct PDF text extraction
		return c.convertPDF(fileID)
	}

	// Delete the temporary converted file when done
	defer func() {
		if err := c.service.Files.Delete(copiedFile.Id).SupportsAllDrives(true).Do(); err != nil {
			if c.verbose {
				log.Printf("Warning: Failed to delete temporary file %s: %v", copiedFile.Id, err)
			}
		}
	}()

	// Export the converted Google Doc as markdown
	body, err := c.executeExportWithRetry(copiedFile.Id, "text/markdown")
	if err != nil {
		return nil, "", fmt.Errorf("failed to export converted document: %w", err)
	}
	defer body.Close()

	content, err := io.ReadAll(body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read converted content: %w", err)
	}

	if c.verbose {
		log.Printf("Successfully converted PDF %s using Google Docs", fileID)
	}

	return content, modifiedTime, nil
}

// convertPDF downloads a PDF and converts it to markdown using direct text extraction (fallback)
func (c *Converter) convertPDF(fileID string) ([]byte, string, error) {
	// Get revision hash
	file, err := c.getFileMetadata(fileID)
	if err != nil {
		return nil, "", err
	}

	// Download PDF
	body, err := c.executeDownloadWithRetry(fileID)
	if err != nil {
		return nil, "", err
	}
	defer body.Close()

	// Create temp file for PDF
	tempFile, err := os.CreateTemp("", "gdrive-*.pdf")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Write PDF to temp file
	if _, err := io.Copy(tempFile, body); err != nil {
		return nil, "", fmt.Errorf("failed to write PDF: %w", err)
	}

	// Close the file so we can read it
	tempFile.Close()

	// Convert PDF to markdown
	content, err := convertPDFToMarkdown(tempFile.Name())
	if err != nil {
		return nil, "", fmt.Errorf("failed to convert PDF to markdown: %w", err)
	}

	return content, file.ModifiedTime, nil
}

// convertPDFToMarkdown converts a PDF file to markdown
func convertPDFToMarkdown(pdfPath string) ([]byte, error) {
	// Open PDF file
	pdfFile, pdfReader, err := pdf.Open(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer pdfFile.Close()

	var sb strings.Builder
	sb.WriteString("# PDF Content\n\n")

	// Extract text from each page
	numPages := pdfReader.NumPage()
	for pageNum := 1; pageNum <= numPages; pageNum++ {
		page := pdfReader.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		// Get page content
		text, err := page.GetPlainText(nil)
		if err != nil {
			log.Printf("Warning: failed to extract text from page %d: %v", pageNum, err)
			continue
		}

		// Add page content
		if text != "" {
			if pageNum > 1 {
				sb.WriteString("\n\n---\n\n") // Page separator
			}
			sb.WriteString(text)
		}
	}

	return []byte(sb.String()), nil
}

// normalizeMultilineURLs fixes Google Drive/Docs URLs that are broken across multiple lines
// and unescapes markdown characters within URLs
// Example: "*https://docs.google.com/document/d/abc*\n*defg/edit*" -> "https://docs.google.com/document/d/abcdefg/edit"
// Example: "https://docs.google.com/document/d/abc\_def/edit" -> "https://docs.google.com/document/d/abc_def/edit"
func normalizeMultilineURLs(content string) string {
	// Step 1: Fix URLs broken across multiple lines FIRST (before unescaping)
	// Pattern to match Google Drive URL that might continue on next line
	// Uses [^\s\n]+? (non-greedy) to allow underscores, backslashes, etc in URLs
	// Explicitly matches and strips markdown markers (*/_) around the line break
	urlContinuationPattern := regexp.MustCompile(
		`(https://(?:drive\.google\.com|docs\.google\.com)/[^\s\n]+?)[\*_]*\s*\n\s*[\*_]*([^\s\n]+?)[\*_]*`,
	)

	// Keep replacing until no more matches (handles multi-line breaks)
	for {
		normalized := urlContinuationPattern.ReplaceAllString(content, "$1$2")
		if normalized == content {
			break
		}
		content = normalized
	}

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

// rewriteLinks rewrites Google Drive/Docs links to relative paths
func (c *Converter) rewriteLinks(content string, sourceRecord *csv.ConversionRecord) string {
	// Normalize content to fix URLs broken across multiple lines
	content = normalizeMultilineURLs(content)

	// Pattern to match Google Drive and Google Docs links
	// Using non-capturing group (?:...) for domain alternation
	linkPattern := regexp.MustCompile(`\[([^\]]+)\]\((https://(?:drive\.google\.com|docs\.google\.com)/[^\)]+)\)`)

	return linkPattern.ReplaceAllStringFunc(content, func(match string) string {
		matches := linkPattern.FindStringSubmatch(match)
		if len(matches) != 3 {
			return match
		}

		linkText := matches[1]
		linkURL := matches[2]

		// Look up target in link map by exact URL first
		targetRecord, exists := c.linkMap[linkURL]

		// If not found by URL, try by file ID (for cross-format matching)
		if !exists {
			targetID, err := extractFileID(linkURL)
			if err != nil {
				return match // Keep original if we can't extract ID
			}
			targetRecord, exists = c.linkMap[targetID]
			if !exists {
				// Not in our inventory - keep original URL as-is
				return match
			}
		}

		// Calculate relative path with normalized filename
		normalizedTargetTitle := normalizeFilename(targetRecord.Title)
		relPath := utils.CalculateRelativePath(
			sourceRecord.GetFragments(),
			targetRecord.GetFragments(),
			normalizedTargetTitle,
		)

		return fmt.Sprintf("[%s](%s)", linkText, relPath)
	})
}

// generateFrontmatter generates YAML frontmatter for the document
func (c *Converter) generateFrontmatter(record *csv.ConversionRecord, revisionHash, content string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("description: %s\n", escapeYAML(record.Title)))
	sb.WriteString("editor: markdown\n")
	sb.WriteString(fmt.Sprintf("hash-gdrive: %s\n", escapeYAML(revisionHash)))
	sb.WriteString(fmt.Sprintf("hash-content: %s\n", utils.CalculateStringHash(content)))
	sb.WriteString("published: true\n")

	tags := record.GetTagsList()
	if len(tags) > 0 {
		sb.WriteString(fmt.Sprintf("tags: %s\n", strings.Join(tags, ", ")))
	}

	sb.WriteString(fmt.Sprintf("title: %s\n", escapeYAML(record.Title)))
	sb.WriteString("---\n")

	return sb.String()
}

// escapeYAML escapes special characters in YAML values
func escapeYAML(s string) string {
	// If string contains special characters, quote it
	if strings.ContainsAny(s, ":#@&*!|>'\"%[]{}") || strings.HasPrefix(s, "-") {
		// Escape quotes
		s = strings.ReplaceAll(s, "\"", "\\\"")
		return fmt.Sprintf("\"%s\"", s)
	}
	return s
}

// getFileMetadata retrieves metadata for a file
func (c *Converter) getFileMetadata(fileID string) (*drive.File, error) {
	maxRetries := 5
	baseDelay := time.Second

	for i := 0; i < maxRetries; i++ {
		file, err := c.service.Files.Get(fileID).
			Fields("id, name, mimeType, modifiedTime").
			SupportsAllDrives(true).
			Do()

		if err == nil {
			return file, nil
		}

		// Check if it's a rate limit error
		if apiErr, ok := err.(*googleapi.Error); ok {
			if apiErr.Code == 403 || apiErr.Code == 429 {
				delay := baseDelay * time.Duration(1<<uint(i))
				if c.verbose {
					log.Printf("Rate limited, retrying in %v...", delay)
				}
				time.Sleep(delay)
				continue
			}
		}

		return nil, err
	}

	// Final attempt
	return c.service.Files.Get(fileID).
		Fields("id, name, mimeType, modifiedTime").
		SupportsAllDrives(true).
		Do()
}

// executeExportWithRetry exports a file with retry logic
func (c *Converter) executeExportWithRetry(fileID, mimeType string) (io.ReadCloser, error) {
	maxRetries := 5
	baseDelay := time.Second

	for i := 0; i < maxRetries; i++ {
		resp, err := c.service.Files.Export(fileID, mimeType).Download()

		if err == nil {
			return resp.Body, nil
		}

		// Check if it's a rate limit error
		if apiErr, ok := err.(*googleapi.Error); ok {
			if apiErr.Code == 403 || apiErr.Code == 429 {
				delay := baseDelay * time.Duration(1<<uint(i))
				if c.verbose {
					log.Printf("Rate limited, retrying in %v...", delay)
				}
				time.Sleep(delay)
				continue
			}
		}

		return nil, err
	}

	// Final attempt
	resp, err := c.service.Files.Export(fileID, mimeType).Download()
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// executeDownloadWithRetry downloads a file with retry logic
func (c *Converter) executeDownloadWithRetry(fileID string) (io.ReadCloser, error) {
	maxRetries := 5
	baseDelay := time.Second

	for i := 0; i < maxRetries; i++ {
		resp, err := c.service.Files.Get(fileID).SupportsAllDrives(true).Download()

		if err == nil {
			return resp.Body, nil
		}

		// Check if it's a rate limit error
		if apiErr, ok := err.(*googleapi.Error); ok {
			if apiErr.Code == 403 || apiErr.Code == 429 {
				delay := baseDelay * time.Duration(1<<uint(i))
				if c.verbose {
					log.Printf("Rate limited, retrying in %v...", delay)
				}
				time.Sleep(delay)
				continue
			}
		}

		return nil, err
	}

	// Final attempt
	resp, err := c.service.Files.Get(fileID).SupportsAllDrives(true).Download()
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// normalizeFilename normalizes a filename to be lowercase, hyphenated, and without special characters
func normalizeFilename(filename string) string {
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

// extractFileID extracts the file ID from a Google Drive URL
func extractFileID(urlStr string) (string, error) {
	// This is duplicated from discovery package for now
	// Could be moved to utils if needed
	var driveIDPattern = regexp.MustCompile(`[-\w]{25,}`)

	// Try to extract ID from various URL formats
	if strings.Contains(urlStr, "/d/") {
		parts := strings.Split(urlStr, "/d/")
		if len(parts) > 1 {
			id := strings.Split(parts[1], "/")[0]
			return id, nil
		}
	}

	// Try pattern matching
	matches := driveIDPattern.FindStringSubmatch(urlStr)
	if len(matches) > 0 {
		return matches[0], nil
	}

	return "", fmt.Errorf("could not extract file ID from URL: %s", urlStr)
}
