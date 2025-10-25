package csv

import (
	"encoding/csv"
	"fmt"
	"os"
)

// WriteDiscoveryCSV writes discovery results to a CSV file
func WriteDiscoveryCSV(filePath string, records []DiscoveryRecord) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create output CSV: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"link", "title"}); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write records
	for _, record := range records {
		if err := writer.Write([]string{record.Link, record.Title}); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return writer.Error()
}
