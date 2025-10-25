package csv

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

// InputRecord represents a record from the input CSV for discovery mode
type InputRecord struct {
	URL string
}

// DiscoveryRecord represents a record for discovery output
type DiscoveryRecord struct {
	Link  string
	Title string
}

// ConversionRecord represents a record from the enhanced CSV for conversion mode
type ConversionRecord struct {
	Link  string
	Title string
	Tags  string
	Frag1 string
	Frag2 string
	Frag3 string
	Frag4 string
	Frag5 string
}

// ParseInputCSV reads the input CSV file for discovery mode
func ParseInputCSV(filePath string) ([]InputRecord, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open input CSV: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Find URL column index
	urlIdx := -1
	for i, col := range header {
		if strings.EqualFold(col, "url") || strings.EqualFold(col, "link") {
			urlIdx = i
			break
		}
	}

	if urlIdx == -1 {
		return nil, fmt.Errorf("no 'url' or 'link' column found in CSV")
	}

	// Read records
	var records []InputRecord
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading CSV row: %w", err)
		}

		if len(row) <= urlIdx {
			continue // Skip malformed rows
		}

		url := strings.TrimSpace(row[urlIdx])
		if url != "" {
			records = append(records, InputRecord{URL: url})
		}
	}

	return records, nil
}

// ParseConversionCSV reads the enhanced CSV file for conversion mode
func ParseConversionCSV(filePath string) ([]ConversionRecord, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open conversion CSV: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Find column indices
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.ToLower(col)] = i
	}

	// Validate required columns
	requiredCols := []string{"link", "title"}
	for _, col := range requiredCols {
		if _, exists := colMap[col]; !exists {
			return nil, fmt.Errorf("required column '%s' not found in CSV", col)
		}
	}

	// Read records
	var records []ConversionRecord
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading CSV row: %w", err)
		}

		record := ConversionRecord{
			Link:  getString(row, colMap["link"]),
			Title: getString(row, colMap["title"]),
			Tags:  getString(row, colMap["tags"]),
			Frag1: getString(row, colMap["frag1"]),
			Frag2: getString(row, colMap["frag2"]),
			Frag3: getString(row, colMap["frag3"]),
			Frag4: getString(row, colMap["frag4"]),
			Frag5: getString(row, colMap["frag5"]),
		}

		if record.Link != "" && record.Title != "" {
			records = append(records, record)
		}
	}

	return records, nil
}

// getString safely gets a string from a row at the given index
func getString(row []string, idx int) string {
	if idx >= 0 && idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

// GetFragments returns the fragments as a slice
func (r *ConversionRecord) GetFragments() []string {
	return []string{r.Frag1, r.Frag2, r.Frag3, r.Frag4, r.Frag5}
}

// GetTagsList returns tags as a slice
func (r *ConversionRecord) GetTagsList() []string {
	if r.Tags == "" {
		return nil
	}
	var tags []string
	for _, tag := range strings.Split(r.Tags, ",") {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}
