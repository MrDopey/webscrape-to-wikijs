package sync

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"google.golang.org/api/drive/v3"

	"github.com/yourusername/webscrape-to-wikijs/internal/csv"
	"github.com/yourusername/webscrape-to-wikijs/internal/utils"
)

// Syncer handles synchronization of existing markdown files with Google Drive
type Syncer struct {
	service      *drive.Service
	outputDir    string
	verbose      bool
	dryRun       bool
	linkMap      map[string]*csv.ConversionRecord // Maps file ID to record
	linkRewriter *LinkRewriter
	mu           sync.Mutex
}

// SyncResult represents the result of syncing a single file
type SyncResult struct {
	FilePath      string
	Status        string // "updated", "unchanged", "error", "skipped"
	Error         error
	OldHash       string
	NewHash       string
	ContentLength int
}

// LinkRewriter handles rewriting Google Drive links to relative paths
type LinkRewriter struct {
	linkMap map[string]*csv.ConversionRecord
}

// NewSyncer creates a new Syncer
func NewSyncer(service *drive.Service, outputDir string, verbose, dryRun bool) *Syncer {
	return &Syncer{
		service:      service,
		outputDir:    outputDir,
		verbose:      verbose,
		dryRun:       dryRun,
		linkMap:      make(map[string]*csv.ConversionRecord),
		linkRewriter: &LinkRewriter{linkMap: make(map[string]*csv.ConversionRecord)},
	}
}

// Sync synchronizes all markdown files in the output directory with Google Drive
func (s *Syncer) Sync(records []csv.ConversionRecord, workers int) ([]SyncResult, error) {
	// Build link map for O(1) lookup
	for i := range records {
		s.linkMap[records[i].Link] = &records[i]
		s.linkRewriter.linkMap[records[i].Link] = &records[i]

		// Also index by file ID
		fileID, err := utils.ExtractFileID(records[i].Link)
		if err != nil {
			log.Printf("Warning: failed to extract file ID from %s: %v", records[i].Link, err)
			continue
		}
		s.linkMap[fileID] = &records[i]
		s.linkRewriter.linkMap[fileID] = &records[i]
	}

	// Find all markdown files in output directory
	markdownFiles, err := s.findMarkdownFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to find markdown files: %w", err)
	}

	if s.verbose {
		log.Printf("Found %d markdown files to check for updates", len(markdownFiles))
	}

	// Create worker pool
	jobs := make(chan string, len(markdownFiles))
	results := make(chan SyncResult, len(markdownFiles))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range jobs {
				result := s.syncFile(filePath)
				results <- result
			}
		}()
	}

	// Send jobs
	for _, filePath := range markdownFiles {
		jobs <- filePath
	}
	close(jobs)

	// Wait for completion
	wg.Wait()
	close(results)

	// Collect results
	var syncResults []SyncResult
	updated := 0
	unchanged := 0
	errors := 0
	skipped := 0

	for result := range results {
		syncResults = append(syncResults, result)
		switch result.Status {
		case "updated":
			updated++
		case "unchanged":
			unchanged++
		case "error":
			errors++
		case "skipped":
			skipped++
		}
	}

	if s.verbose || errors > 0 {
		log.Printf("Sync complete: %d updated, %d unchanged, %d skipped, %d errors", updated, unchanged, skipped, errors)
	}

	return syncResults, nil
}

// findMarkdownFiles finds all markdown files in the output directory
func (s *Syncer) findMarkdownFiles() ([]string, error) {
	var files []string

	err := filepath.Walk(s.outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// syncFile syncs a single markdown file
func (s *Syncer) syncFile(filePath string) SyncResult {
	result := SyncResult{
		FilePath: filePath,
		Status:   "unchanged",
	}

	// Read existing file
	content, err := os.ReadFile(filePath)
	if err != nil {
		result.Status = "error"
		result.Error = fmt.Errorf("failed to read file: %w", err)
		return result
	}

	// Parse frontmatter
	frontmatter, _, err := s.parseFrontmatter(string(content))
	if err != nil {
		result.Status = "error"
		result.Error = fmt.Errorf("failed to parse frontmatter: %w", err)
		return result
	}

	// Check if this is a stub document
	oldHash, hasHash := frontmatter["hash-gdrive"]
	if !hasHash {
		result.Status = "skipped"
		result.Error = fmt.Errorf("no hash-gdrive field found")
		return result
	}

	// Skip stub documents
	if oldHash == "stub" {
		result.Status = "skipped"
		if s.verbose {
			log.Printf("Skipping stub document: %s", filePath)
		}
		return result
	}

	// Get Google Drive link
	gdriveLink, hasLink := frontmatter["gdrive-link"]
	if !hasLink {
		result.Status = "skipped"
		result.Error = fmt.Errorf("no gdrive-link field found")
		return result
	}

	result.OldHash = oldHash

	// Extract file ID
	fileID, err := utils.ExtractFileID(gdriveLink)
	if err != nil {
		result.Status = "error"
		result.Error = fmt.Errorf("failed to extract file ID: %w", err)
		return result
	}

	// Get current metadata from Google Drive
	file, err := s.getFileMetadata(fileID)
	if err != nil {
		result.Status = "error"
		result.Error = fmt.Errorf("failed to get file metadata: %w", err)
		return result
	}

	result.NewHash = file.ModifiedTime

	// Check if file has been updated
	if oldHash == file.ModifiedTime {
		result.Status = "unchanged"
		if s.verbose {
			log.Printf("No changes: %s", filePath)
		}
		return result
	}

	// File has been updated - fetch new content
	if s.verbose {
		log.Printf("Updating: %s (old: %s, new: %s)", filePath, oldHash, file.ModifiedTime)
	}

	// Export new content
	newContent, err := s.exportDocument(fileID, file.MimeType)
	if err != nil {
		result.Status = "error"
		result.Error = fmt.Errorf("failed to export document: %w", err)
		return result
	}

	// Get record for link rewriting context
	record := s.linkMap[fileID]
	if record == nil {
		result.Status = "error"
		result.Error = fmt.Errorf("record not found in link map")
		return result
	}

	// Rewrite links in new content
	newContentStr := s.linkRewriter.RewriteLinks(string(newContent), record)

	// Build content with preamble (matching convert behavior)
	preamble := fmt.Sprintf("> Link: %s", gdriveLink)
	contentWithPreamble := preamble + "\n\n" + newContentStr

	// Update frontmatter
	frontmatter["hash-gdrive"] = file.ModifiedTime
	frontmatter["hash-content"] = utils.CalculateStringHash(contentWithPreamble)

	// Reconstruct file
	finalContent := s.buildFrontmatter(frontmatter) + "\n" + contentWithPreamble

	result.ContentLength = len(finalContent)

	if s.dryRun {
		log.Printf("Would update: %s", filePath)
		result.Status = "updated"
		return result
	}

	// Write updated file
	if err := os.WriteFile(filePath, []byte(finalContent), 0644); err != nil {
		result.Status = "error"
		result.Error = fmt.Errorf("failed to write file: %w", err)
		return result
	}

	result.Status = "updated"
	if s.verbose {
		log.Printf("Updated: %s", filePath)
	}

	return result
}

// parseFrontmatter parses YAML frontmatter from markdown content
func (s *Syncer) parseFrontmatter(content string) (map[string]string, string, error) {
	frontmatter := make(map[string]string)

	// Check for frontmatter markers
	if !strings.HasPrefix(content, "---\n") {
		return nil, "", fmt.Errorf("no frontmatter found")
	}

	// Find end of frontmatter
	endIdx := strings.Index(content[4:], "\n---\n")
	if endIdx == -1 {
		return nil, "", fmt.Errorf("frontmatter not closed")
	}

	// Extract frontmatter and body
	frontmatterStr := content[4 : endIdx+4]
	body := content[endIdx+9:]

	// Parse frontmatter lines
	lines := strings.Split(frontmatterStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split on first colon
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		value = strings.Trim(value, "\"")

		frontmatter[key] = value
	}

	return frontmatter, body, nil
}

// buildFrontmatter builds YAML frontmatter from a map
func (s *Syncer) buildFrontmatter(fm map[string]string) string {
	var sb strings.Builder
	sb.WriteString("---\n")

	// Write fields in a consistent order
	order := []string{"description", "editor", "gdrive-link", "hash-gdrive", "hash-content", "published", "tags", "title"}
	for _, key := range order {
		if value, exists := fm[key]; exists {
			// Quote values that might contain special characters
			if strings.ContainsAny(value, ":#@&*!|>'\"%[]{}") || strings.HasPrefix(value, "-") {
				value = strings.ReplaceAll(value, "\"", "\\\"")
				sb.WriteString(fmt.Sprintf("%s: \"%s\"\n", key, value))
			} else {
				sb.WriteString(fmt.Sprintf("%s: %s\n", key, value))
			}
		}
	}

	sb.WriteString("---\n")
	return sb.String()
}

// getFileMetadata retrieves metadata for a file
func (s *Syncer) getFileMetadata(fileID string) (*drive.File, error) {
	file, err := s.service.Files.Get(fileID).
		Fields("id, name, mimeType, modifiedTime").
		SupportsAllDrives(true).
		Do()

	if err != nil {
		return nil, fmt.Errorf("failed to get file metadata: %w", err)
	}

	return file, nil
}

// exportDocument exports a Google Workspace document as markdown
func (s *Syncer) exportDocument(fileID, mimeType string) ([]byte, error) {
	if !strings.HasPrefix(mimeType, "application/vnd.google-apps.") {
		return nil, fmt.Errorf("unsupported MIME type: %s", mimeType)
	}

	resp, err := s.service.Files.Export(fileID, "text/markdown").Download()
	if err != nil {
		return nil, fmt.Errorf("failed to export document: %w", err)
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return content, nil
}

// RewriteLinks rewrites Google Drive/Docs links to relative paths
func (lr *LinkRewriter) RewriteLinks(content string, sourceRecord *csv.ConversionRecord) string {
	// Normalize content to fix URLs broken across multiple lines
	content = utils.NormalizeMultilineURLs(content)

	// Pattern to match Google Drive and Google Docs links
	linkPattern := regexp.MustCompile(`\[([^\]]+)\]\((https://(?:drive\.google\.com|docs\.google\.com)/[^\)]+)\)`)

	return linkPattern.ReplaceAllStringFunc(content, func(match string) string {
		matches := linkPattern.FindStringSubmatch(match)
		if len(matches) != 3 {
			return match
		}

		linkText := matches[1]
		linkURL := matches[2]

		// Look up target in link map by exact URL first
		targetRecord, exists := lr.linkMap[linkURL]

		// If not found by URL, try by file ID (for cross-format matching)
		if !exists {
			targetID, err := utils.ExtractFileID(linkURL)
			if err != nil {
				return match // Keep original if we can't extract ID
			}
			targetRecord, exists = lr.linkMap[targetID]
			if !exists {
				// Not in our inventory - keep original URL as-is
				return match
			}
		}

		// Calculate relative path with normalized filename
		normalizedTargetTitle := utils.NormalizeFilename(targetRecord.Title)
		relPath := utils.CalculateRelativePath(
			sourceRecord.GetFragments(),
			targetRecord.GetFragments(),
			normalizedTargetTitle,
		)

		return fmt.Sprintf("[%s](%s)", linkText, relPath)
	})
}
