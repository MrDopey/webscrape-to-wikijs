package discovery

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"

	"github.com/yourusername/webscrape-to-wikijs/internal/csv"
)

var (
	// Regex patterns for extracting file/folder IDs from URLs
	driveIDPattern = regexp.MustCompile(`[-\w]{25,}`)
)

// Discoverer handles discovery of files in Google Drive
type Discoverer struct {
	service  *drive.Service
	verbose  bool
	maxDepth int
	mu       sync.Mutex
	seen     map[string]bool // Track seen file IDs to avoid duplicates
	depth    map[string]int  // Track depth level for each file
}

// NewDiscoverer creates a new Discoverer
func NewDiscoverer(service *drive.Service, verbose bool, maxDepth int) *Discoverer {
	return &Discoverer{
		service:  service,
		verbose:  verbose,
		maxDepth: maxDepth,
		seen:     make(map[string]bool),
		depth:    make(map[string]int),
	}
}

// DiscoverFromURLs discovers all files from a list of URLs
func (d *Discoverer) DiscoverFromURLs(urls []string) ([]csv.DiscoveryRecord, error) {
	var records []csv.DiscoveryRecord
	var mu sync.Mutex

	for _, urlStr := range urls {
		fileID, err := extractFileID(urlStr)
		if err != nil {
			// Invalid URL or malformed file ID - mark as invalid
			log.Printf("Warning: invalid URL or file ID in %s: %v", urlStr, err)
			record := csv.DiscoveryRecord{
				Link:   urlStr,
				Title:  "INVALID_URL",
				Status: "invalid",
			}
			mu.Lock()
			records = append(records, record)
			mu.Unlock()
			continue
		}

		// Discover from this file/folder at depth 0, preserving original URL
		fileRecords, err := d.discoverFromFileIDWithURL(fileID, urlStr, 0)
		if err != nil {
			log.Printf("Warning: failed to discover %s: %v", fileID, err)
			continue
		}

		mu.Lock()
		records = append(records, fileRecords...)
		mu.Unlock()
	}

	return records, nil
}

// discoverFromFileID discovers a file and recursively follows links within it
func (d *Discoverer) discoverFromFileID(fileID string, currentDepth int) ([]csv.DiscoveryRecord, error) {
	return d.discoverFromFileIDWithURL(fileID, "", currentDepth)
}

// discoverFromFileIDWithURL discovers a file with an optional original URL and recursively follows links within it
func (d *Discoverer) discoverFromFileIDWithURL(fileID string, originalURL string, currentDepth int) ([]csv.DiscoveryRecord, error) {
	var records []csv.DiscoveryRecord

	// Check if already seen
	d.mu.Lock()
	if d.seen[fileID] {
		d.mu.Unlock()
		return records, nil
	}
	d.seen[fileID] = true
	d.depth[fileID] = currentDepth
	d.mu.Unlock()

	// Get file metadata
	file, err := d.getFileMetadata(fileID)
	if err != nil {
		// Determine error type
		status := determineErrorStatus(err)
		log.Printf("Warning: file %s status: %s (%v)", fileID, status, err)
		// Use original URL if available, otherwise construct one
		link := originalURL
		if link == "" {
			link = buildFileLink(fileID, "")
		}
		return []csv.DiscoveryRecord{{
			Link:   link,
			Title:  fileID,
			Status: status,
		}}, nil
	}

	if d.verbose {
		log.Printf("Processing: %s (%s) at depth %d", file.Name, file.MimeType, currentDepth)
	}

	if file.MimeType == "application/vnd.google-apps.folder" {
		// Recursively discover folder contents
		folderRecords, err := d.discoverFolder(fileID)
		if err != nil {
			log.Printf("Warning: failed to discover folder %s: %v", fileID, err)
		}
		records = append(records, folderRecords...)
	} else {
		// Add this file to records
		// Use original URL if available, otherwise construct one based on MIME type
		link := originalURL
		if link == "" {
			link = buildFileLink(fileID, file.MimeType)
		}
		records = append(records, csv.DiscoveryRecord{
			Link:   link,
			Title:  file.Name,
			Status: "available",
		})

		// If we haven't reached max depth, discover links within the document
		if currentDepth < d.maxDepth {
			linkedURLs := d.extractLinksFromDocument(fileID, file.MimeType)
			for _, linkedURL := range linkedURLs {
				linkedID, err := extractFileID(linkedURL)
				if err != nil {
					log.Printf("Warning: failed to extract file ID from %s: %v", linkedURL, err)
					continue
				}
				linkedRecords, err := d.discoverFromFileIDWithURL(linkedID, linkedURL, currentDepth+1)
				if err != nil {
					log.Printf("Warning: failed to discover linked file %s: %v", linkedID, err)
					continue
				}
				records = append(records, linkedRecords...)
			}
		} else if d.verbose && currentDepth >= d.maxDepth {
			log.Printf("Max depth %d reached for %s, skipping link discovery", d.maxDepth, file.Name)
		}
	}

	return records, nil
}

// discoverFolder recursively discovers all files in a folder
func (d *Discoverer) discoverFolder(folderID string) ([]csv.DiscoveryRecord, error) {
	var records []csv.DiscoveryRecord

	// Check if we've already processed this folder
	d.mu.Lock()
	if d.seen[folderID] {
		d.mu.Unlock()
		return records, nil
	}
	d.seen[folderID] = true
	d.mu.Unlock()

	pageToken := ""
	for {
		query := fmt.Sprintf("'%s' in parents and trashed = false", folderID)
		call := d.service.Files.List().
			Q(query).
			Fields("nextPageToken, files(id, name, mimeType)").
			PageSize(100).
			SupportsAllDrives(true).
			IncludeItemsFromAllDrives(true)

		if pageToken != "" {
			call.PageToken(pageToken)
		}

		res, err := d.executeFileListWithRetry(func() (*drive.FileList, error) {
			return call.Do()
		})

		if err != nil {
			return nil, fmt.Errorf("failed to list files in folder %s: %w", folderID, err)
		}

		for _, file := range res.Files {
			d.mu.Lock()
			if d.seen[file.Id] {
				d.mu.Unlock()
				continue
			}
			d.seen[file.Id] = true
			d.mu.Unlock()

			if d.verbose {
				log.Printf("Found: %s (%s)", file.Name, file.MimeType)
			}

			if file.MimeType == "application/vnd.google-apps.folder" {
				// Recursively process subfolder
				subRecords, err := d.discoverFolder(file.Id)
				if err != nil {
					log.Printf("Warning: failed to discover subfolder %s: %v", file.Id, err)
					continue
				}
				records = append(records, subRecords...)
			} else {
				// Add file record - mark as available since we successfully retrieved it
				records = append(records, csv.DiscoveryRecord{
					Link:   buildFileLink(file.Id, file.MimeType),
					Title:  file.Name,
					Status: "available",
				})
			}
		}

		pageToken = res.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return records, nil
}

// getFileMetadata retrieves metadata for a file
func (d *Discoverer) getFileMetadata(fileID string) (*drive.File, error) {
	file, err := d.executeFileWithRetry(func() (*drive.File, error) {
		return d.service.Files.Get(fileID).
			Fields("id, name, mimeType").
			SupportsAllDrives(true).
			Do()
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get file metadata: %w", err)
	}

	return file, nil
}

// executeFileListWithRetry executes a FileList function with exponential backoff retry
func (d *Discoverer) executeFileListWithRetry(fn func() (*drive.FileList, error)) (*drive.FileList, error) {
	maxRetries := 5
	baseDelay := time.Second

	for i := 0; i < maxRetries; i++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		// Check if it's a rate limit error
		if apiErr, ok := err.(*googleapi.Error); ok {
			if apiErr.Code == 403 || apiErr.Code == 429 {
				delay := baseDelay * time.Duration(1<<uint(i))
				if d.verbose {
					log.Printf("Rate limited, retrying in %v...", delay)
				}
				time.Sleep(delay)
				continue
			}
		}

		return nil, err
	}

	return fn() // Final attempt
}

// executeFileWithRetry executes a File function with exponential backoff retry
func (d *Discoverer) executeFileWithRetry(fn func() (*drive.File, error)) (*drive.File, error) {
	maxRetries := 5
	baseDelay := time.Second

	for i := 0; i < maxRetries; i++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		// Check if it's a rate limit error
		if apiErr, ok := err.(*googleapi.Error); ok {
			if apiErr.Code == 403 || apiErr.Code == 429 {
				delay := baseDelay * time.Duration(1<<uint(i))
				if d.verbose {
					log.Printf("Rate limited, retrying in %v...", delay)
				}
				time.Sleep(delay)
				continue
			}
		}

		return nil, err
	}

	return fn() // Final attempt
}

// normalizeMultilineURLs fixes Google Drive/Docs URLs that are broken across multiple lines
// Example: "*https://docs.google.com/document/d/abc*\n*defg/edit*" -> "https://docs.google.com/document/d/abcdefg/edit"
func normalizeMultilineURLs(content string) string {
	// Pattern to match Google Drive URL that might continue on next line
	// Captures: URL (without trailing markdown), then matches markdown/whitespace/newline, then captures continuation
	// The [^\s\*_\n]+ ensures we don't capture markdown formatting or whitespace as part of the URL
	urlContinuationPattern := regexp.MustCompile(
		`(https://(?:drive\.google\.com|docs\.google\.com)/[^\s\*_\n]+)[\*_]?\s*\n\s*[\*_]?([^\s\*_\n]+)[\*_]?`,
	)

	// Keep replacing until no more matches (handles multi-line breaks)
	for {
		normalized := urlContinuationPattern.ReplaceAllString(content, "$1$2")
		if normalized == content {
			break
		}
		content = normalized
	}

	return content
}

// extractLinksFromDocument exports a document and extracts Google Drive/Docs URLs
func (d *Discoverer) extractLinksFromDocument(fileID, mimeType string) []string {
	var linkedURLs []string
	var content []byte
	var err error

	// Handle PDFs by converting to Google Docs format
	if mimeType == "application/pdf" {
		content, err = d.extractLinksFromPDF(fileID)
		if err != nil {
			if d.verbose {
				log.Printf("Warning: failed to extract links from PDF %s: %v", fileID, err)
			}
			return linkedURLs
		}
	} else if strings.HasPrefix(mimeType, "application/vnd.google-apps.") {
		// Skip folders
		if mimeType == "application/vnd.google-apps.folder" {
			return linkedURLs
		}

		// Export Google Workspace document as markdown to search for links
		resp, err := d.service.Files.Export(fileID, "text/markdown").Download()
		if err != nil {
			if d.verbose {
				log.Printf("Warning: failed to export %s for link extraction: %v", fileID, err)
			}
			return linkedURLs
		}
		defer resp.Body.Close()

		// Read content
		content, err = io.ReadAll(resp.Body)
		if err != nil {
			if d.verbose {
				log.Printf("Warning: failed to read content of %s: %v", fileID, err)
			}
			return linkedURLs
		}
	} else {
		// Unsupported file type for link extraction
		return linkedURLs
	}

	// Normalize content to fix URLs broken across multiple lines
	normalizedContent := normalizeMultilineURLs(string(content))

	// Find all Google Drive/Docs URLs in the content
	// Pattern matches both drive.google.com and docs.google.com URLs
	linkPattern := regexp.MustCompile(`https://(?:drive\.google\.com|docs\.google\.com)/[^\s\)]+`)
	matches := linkPattern.FindAllString(normalizedContent, -1)

	// Process URLs and preserve them
	for _, urlStr := range matches {
		id, err := extractFileID(urlStr)
		if err != nil {
			continue // Skip invalid URLs
		}

		// Check against global seen map to avoid re-processing
		d.mu.Lock()
		alreadySeen := d.seen[id]
		d.mu.Unlock()

		// Avoid duplicates and self-references
		if !alreadySeen && id != fileID {
			linkedURLs = append(linkedURLs, urlStr)
		}
	}

	if d.verbose && len(linkedURLs) > 0 {
		log.Printf("Found %d new linked documents in %s", len(linkedURLs), fileID)
	}

	return linkedURLs
}

// extractLinksFromPDF converts a PDF to Google Docs format and extracts its content for link discovery
func (d *Discoverer) extractLinksFromPDF(fileID string) ([]byte, error) {
	if d.verbose {
		log.Printf("Converting PDF %s to Google Docs for link extraction...", fileID)
	}

	// Create a copy of the PDF as a Google Doc
	// This mimics the "Open with Google Docs" behavior in the UI
	copyFile := &drive.File{
		Name:     "temp_link_extraction_" + fileID,
		MimeType: "application/vnd.google-apps.document",
	}

	copiedFile, err := d.service.Files.Copy(fileID, copyFile).SupportsAllDrives(true).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to convert PDF to Google Docs: %w", err)
	}

	// Delete the temporary converted file when done
	defer func() {
		if err := d.service.Files.Delete(copiedFile.Id).SupportsAllDrives(true).Do(); err != nil {
			if d.verbose {
				log.Printf("Warning: Failed to delete temporary file %s: %v", copiedFile.Id, err)
			}
		}
	}()

	// Export the converted Google Doc as markdown
	resp, err := d.service.Files.Export(copiedFile.Id, "text/markdown").Download()
	if err != nil {
		return nil, fmt.Errorf("failed to export converted document: %w", err)
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read converted content: %w", err)
	}

	if d.verbose {
		log.Printf("Successfully extracted links from PDF %s using Google Docs conversion", fileID)
	}

	return content, nil
}

// extractFileID extracts the file/folder ID from a Google Drive URL
func extractFileID(urlStr string) (string, error) {
	log.Printf("{%s}", urlStr)
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
	parts := strings.Split(u.Path, "/")
	for i, part := range parts {
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

	return "", fmt.Errorf("could not extract file ID from URL")
}

// buildFileLink constructs an appropriate Google link from an ID and MIME type
func buildFileLink(fileID string, mimeType string) string {
	switch mimeType {
	case "application/vnd.google-apps.document":
		return fmt.Sprintf("https://docs.google.com/document/d/%s/edit", fileID)
	case "application/vnd.google-apps.spreadsheet":
		return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit", fileID)
	case "application/vnd.google-apps.presentation":
		return fmt.Sprintf("https://docs.google.com/presentation/d/%s/edit", fileID)
	case "application/vnd.google-apps.form":
		return fmt.Sprintf("https://docs.google.com/forms/d/%s/edit", fileID)
	case "application/vnd.google-apps.drawing":
		return fmt.Sprintf("https://docs.google.com/drawings/d/%s/edit", fileID)
	case "application/vnd.google-apps.folder":
		return fmt.Sprintf("https://drive.google.com/drive/folders/%s", fileID)
	default:
		// For non-Google Workspace files (PDFs, images, etc.)
		return fmt.Sprintf("https://drive.google.com/file/d/%s/view", fileID)
	}
}

// determineErrorStatus determines the status based on the API error
func determineErrorStatus(err error) string {
	if apiErr, ok := err.(*googleapi.Error); ok {
		switch apiErr.Code {
		case 404:
			// File not found - either deleted or never existed
			return "deleted"
		case 403:
			// Permission denied - file exists but no access
			return "permission_denied"
		case 400:
			// Bad request - likely invalid file ID format
			return "invalid"
		default:
			// Other errors
			return "error"
		}
	}
	// Non-API errors
	return "error"
}
