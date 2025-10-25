package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/yourusername/webscrape-to-wikijs/internal/auth"
	"github.com/yourusername/webscrape-to-wikijs/internal/conversion"
	csvpkg "github.com/yourusername/webscrape-to-wikijs/internal/csv"
	"github.com/yourusername/webscrape-to-wikijs/internal/discovery"
)

const (
	usageMessage = `Google Drive Documentation Crawler

Usage:
  gdrive-crawler <command> [flags]

Commands:
  discover   Discover files in Google Drive folders and output CSV
  convert    Convert Google Drive documents to markdown

Discover Flags:
  -input string
        Input CSV file with Google Drive URLs (required)
  -output string
        Output CSV file path (required)
  -credentials string
        Google API credentials JSON file (required)
  -depth int
        Maximum depth for recursive link discovery (default: 5)
  -verbose
        Enable verbose logging

Convert Flags:
  -input string
        Input CSV file with link, title, tags, frag1-5 columns (required)
  -output string
        Output directory path (default: ./output)
  -credentials string
        Google API credentials JSON file (required)
  -workers int
        Number of concurrent workers (default: 5)
  -verbose
        Enable verbose logging
  -dry-run
        Preview actions without writing files

Examples:
  # Discover files
  gdrive-crawler discover -input folders.csv -output links.csv -credentials creds.json

  # Convert documents
  gdrive-crawler convert -input enhanced-links.csv -output ./docs -credentials creds.json -workers 10
`
)

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usageMessage)
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "discover":
		runDiscover()
	case "convert":
		runConvert()
	case "help", "-h", "--help":
		fmt.Print(usageMessage)
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		fmt.Print(usageMessage)
		os.Exit(1)
	}
}

func runDiscover() {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	input := fs.String("input", "", "Input CSV file with Google Drive URLs (required)")
	output := fs.String("output", "", "Output CSV file path (required)")
	credentials := fs.String("credentials", "", "Google API credentials JSON file (required)")
	depth := fs.Int("depth", 5, "Maximum depth for recursive link discovery (default: 5)")
	verbose := fs.Bool("verbose", false, "Enable verbose logging")

	fs.Parse(os.Args[2:])

	// Validate required flags
	if *input == "" || *output == "" || *credentials == "" {
		fmt.Println("Error: -input, -output, and -credentials are required")
		fs.PrintDefaults()
		os.Exit(1)
	}

	// Create context
	ctx := context.Background()

	// Authenticate
	if *verbose {
		log.Println("Authenticating with Google Drive API...")
	}
	driveService, err := auth.NewDriveService(ctx, *credentials)
	if err != nil {
		log.Fatalf("Failed to authenticate: %v", err)
	}

	// Parse input CSV
	if *verbose {
		log.Printf("Reading input from %s...", *input)
	}
	inputRecords, err := csvpkg.ParseInputCSV(*input)
	if err != nil {
		log.Fatalf("Failed to parse input CSV: %v", err)
	}

	// Extract URLs
	var urls []string
	for _, record := range inputRecords {
		urls = append(urls, record.URL)
	}

	if *verbose {
		log.Printf("Found %d URLs to process", len(urls))
	}

	// Discover files
	discoverer := discovery.NewDiscoverer(driveService.Service, *verbose, *depth)
	records, err := discoverer.DiscoverFromURLs(urls)
	if err != nil {
		log.Fatalf("Discovery failed: %v", err)
	}

	if *verbose {
		log.Printf("Discovered %d files", len(records))
	}

	// Write output CSV
	if *verbose {
		log.Printf("Writing output to %s...", *output)
	}
	if err := csvpkg.WriteDiscoveryCSV(*output, records); err != nil {
		log.Fatalf("Failed to write output CSV: %v", err)
	}

	log.Printf("Successfully discovered %d files. Output written to %s", len(records), *output)
}

func runConvert() {
	fs := flag.NewFlagSet("convert", flag.ExitOnError)
	input := fs.String("input", "", "Input CSV file (required)")
	output := fs.String("output", "./output", "Output directory path")
	credentials := fs.String("credentials", "", "Google API credentials JSON file (required)")
	workers := fs.Int("workers", 5, "Number of concurrent workers")
	verbose := fs.Bool("verbose", false, "Enable verbose logging")
	dryRun := fs.Bool("dry-run", false, "Preview actions without writing files")

	fs.Parse(os.Args[2:])

	// Validate required flags
	if *input == "" || *credentials == "" {
		fmt.Println("Error: -input and -credentials are required")
		fs.PrintDefaults()
		os.Exit(1)
	}

	// Create context
	ctx := context.Background()

	// Authenticate
	if *verbose {
		log.Println("Authenticating with Google Drive API...")
	}
	driveService, err := auth.NewDriveService(ctx, *credentials)
	if err != nil {
		log.Fatalf("Failed to authenticate: %v", err)
	}

	// Parse input CSV
	if *verbose {
		log.Printf("Reading input from %s...", *input)
	}
	records, err := csvpkg.ParseConversionCSV(*input)
	if err != nil {
		log.Fatalf("Failed to parse input CSV: %v", err)
	}

	if *verbose {
		log.Printf("Found %d records to convert", len(records))
	}

	// Convert documents
	converter := conversion.NewConverter(driveService.Service, *output, *verbose, *dryRun)
	if err := converter.Convert(records, *workers); err != nil {
		log.Printf("Conversion completed with errors: %v", err)
		os.Exit(1)
	}

	if *dryRun {
		log.Println("Dry run completed successfully")
	} else {
		log.Printf("Successfully converted %d documents to %s", len(records), *output)
	}
}
