package discovery

import (
	"fmt"
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
	service *drive.Service
	verbose bool
	mu      sync.Mutex
	seen    map[string]bool // Track seen file IDs to avoid duplicates
}

// NewDiscoverer creates a new Discoverer
func NewDiscoverer(service *drive.Service, verbose bool) *Discoverer {
	return &Discoverer{
		service: service,
		verbose: verbose,
		seen:    make(map[string]bool),
	}
}

// DiscoverFromURLs discovers all files from a list of URLs
func (d *Discoverer) DiscoverFromURLs(urls []string) ([]csv.DiscoveryRecord, error) {
	var records []csv.DiscoveryRecord
	var mu sync.Mutex

	for _, urlStr := range urls {
		fileID, err := extractFileID(urlStr)
		if err != nil {
			log.Printf("Warning: failed to extract file ID from %s: %v", urlStr, err)
			continue
		}

		// Check if it's a folder or file
		file, err := d.getFileMetadata(fileID)
		if err != nil {
			// File is deleted or inaccessible - still index it with "deleted" status
			log.Printf("Warning: file %s is deleted or inaccessible: %v", fileID, err)
			record := csv.DiscoveryRecord{
				Link:   buildFileLink(fileID),
				Title:  fileID, // Use file ID as title since we can't get the name
				Status: "deleted",
			}
			mu.Lock()
			records = append(records, record)
			mu.Unlock()
			continue
		}

		if d.verbose {
			log.Printf("Processing: %s (%s)", file.Name, file.MimeType)
		}

		if file.MimeType == "application/vnd.google-apps.folder" {
			// Recursively discover folder contents
			folderRecords, err := d.discoverFolder(fileID)
			if err != nil {
				log.Printf("Warning: failed to discover folder %s: %v", fileID, err)
				continue
			}
			mu.Lock()
			records = append(records, folderRecords...)
			mu.Unlock()
		} else {
			// Single file - mark as available
			record := csv.DiscoveryRecord{
				Link:   buildFileLink(fileID),
				Title:  file.Name,
				Status: "available",
			}
			mu.Lock()
			records = append(records, record)
			mu.Unlock()
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

// buildFileLink constructs a Google Drive file link from an ID
func buildFileLink(fileID string) string {
	return fmt.Sprintf("https://drive.google.com/file/d/%s/view", fileID)
}
