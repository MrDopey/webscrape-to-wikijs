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

		// Discover from this file/folder at depth 0
		fileRecords, err := d.discoverFromFileID(fileID, 0)
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
		return []csv.DiscoveryRecord{{
			Link:   buildFileLink(fileID),
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
		records = append(records, csv.DiscoveryRecord{
			Link:   buildFileLink(fileID),
			Title:  file.Name,
			Status: "available",
		})

		// If we haven't reached max depth, discover links within the document
		if currentDepth < d.maxDepth {
			linkedIDs := d.extractLinksFromDocument(fileID, file.MimeType)
			for _, linkedID := range linkedIDs {
				linkedRecords, err := d.discoverFromFileID(linkedID, currentDepth+1)
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
			PageSize(100)

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
					Link:   buildFileLink(file.Id),
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

// extractLinksFromDocument exports a document and extracts Google Drive/Docs file IDs
func (d *Discoverer) extractLinksFromDocument(fileID, mimeType string) []string {
	var linkedIDs []string

	// Only process Google Workspace documents (not PDFs or other files)
	if !strings.HasPrefix(mimeType, "application/vnd.google-apps.") {
		return linkedIDs
	}

	// Skip folders
	if mimeType == "application/vnd.google-apps.folder" {
		return linkedIDs
	}

	// Export document as plain text to search for links
	resp, err := d.service.Files.Export(fileID, "text/plain").Download()
	if err != nil {
		if d.verbose {
			log.Printf("Warning: failed to export %s for link extraction: %v", fileID, err)
		}
		return linkedIDs
	}
	defer resp.Body.Close()

	// Read content
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		if d.verbose {
			log.Printf("Warning: failed to read content of %s: %v", fileID, err)
		}
		return linkedIDs
	}

	// Find all Google Drive/Docs URLs in the content
	// Pattern matches both drive.google.com and docs.google.com URLs
	linkPattern := regexp.MustCompile(`https://(?:drive\.google\.com|docs\.google\.com)/[^\s\)]+`)
	matches := linkPattern.FindAllString(string(content), -1)

	// Extract file IDs from URLs
	seenIDs := make(map[string]bool)
	for _, urlStr := range matches {
		id, err := extractFileID(urlStr)
		if err != nil {
			continue // Skip invalid URLs
		}

		// Avoid duplicates
		if !seenIDs[id] && id != fileID { // Don't link to self
			seenIDs[id] = true
			linkedIDs = append(linkedIDs, id)
		}
	}

	if d.verbose && len(linkedIDs) > 0 {
		log.Printf("Found %d linked documents in %s", len(linkedIDs), fileID)
	}

	return linkedIDs
}

// extractFileID extracts the file/folder ID from a Google Drive URL
func extractFileID(urlStr string) (string, error) {
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

// buildFileLink constructs a Google Docs link from an ID
func buildFileLink(fileID string) string {
	return fmt.Sprintf("https://docs.google.com/document/d/%s/edit", fileID)
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
