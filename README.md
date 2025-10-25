# Google Drive Documentation Crawler

A comprehensive Go-based CLI tool for crawling Google Drive folders and converting documents into a structured markdown documentation site. The tool operates in two modes: discovery and conversion.

## Features

- **Discovery Mode**: Recursively crawl Google Drive folders and extract all document links with titles
- **Deleted File Tracking**: Automatically indexes deleted/inaccessible files with "deleted" status for documentation tracking
- **Conversion Mode**: Convert Google Drive documents to markdown with intelligent link rewriting and frontmatter
- **Multiple File Types**: Supports Google Docs (native markdown export) and PDFs (text extraction)
- **Smart Link Rewriting**: Automatically converts absolute Google Drive links to relative markdown paths
- **Hierarchical Organization**: Creates nested directory structures based on fragment columns
- **YAML Frontmatter**: Generates metadata including hashes, tags, and publication status
- **Concurrent Processing**: Worker pool for parallel document conversion
- **Rate Limiting**: Built-in exponential backoff for Google Drive API rate limits
- **Robust Error Handling**: Graceful failures with detailed logging

## Installation

### Prerequisites

- Go 1.21 or higher
- Google Cloud Project with Drive API enabled
- Service account credentials or OAuth2 credentials

### Build from Source

```bash
git clone https://github.com/yourusername/webscrape-to-wikijs.git
cd webscrape-to-wikijs
go build -o gdrive-crawler ./cmd/gdrive-crawler
```

## Google Cloud Setup

### 1. Create a Google Cloud Project

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select an existing one
3. Enable the Google Drive API:
   - Navigate to "APIs & Services" > "Library"
   - Search for "Google Drive API"
   - Click "Enable"

### 2. Create Credentials

#### Option A: Service Account (Recommended for automation)

1. Go to "APIs & Services" > "Credentials"
2. Click "Create Credentials" > "Service Account"
3. Fill in the service account details
4. Click "Create and Continue"
5. Grant the service account the "Viewer" role
6. Click "Done"
7. Click on the created service account
8. Go to the "Keys" tab
9. Click "Add Key" > "Create new key"
10. Select "JSON" format
11. Save the downloaded JSON file as `credentials.json`

**Important**: Share your Google Drive folders with the service account email address (found in the JSON file) to grant access.

#### Option B: OAuth2 (For personal use)

1. Go to "APIs & Services" > "Credentials"
2. Click "Create Credentials" > "OAuth client ID"
3. Configure the OAuth consent screen if prompted
4. Select "Desktop app" as the application type
5. Download the credentials JSON file
6. Save it as `credentials.json`

## Usage

### Mode 1: Discovery

Discover all files in Google Drive folders and output a CSV with links and titles.

```bash
./gdrive-crawler discover \
  -input folders.csv \
  -output links.csv \
  -credentials credentials.json \
  -verbose
```

**Input CSV Format** (`folders.csv`):
```csv
url
https://drive.google.com/drive/folders/YOUR_FOLDER_ID
https://drive.google.com/file/d/YOUR_FILE_ID/view
```

**Output CSV Format** (`links.csv`):
```csv
link,title,status
https://drive.google.com/file/d/FILE_ID_1/view,Document Title 1,available
https://drive.google.com/file/d/FILE_ID_2/view,Document Title 2,available
https://drive.google.com/file/d/FILE_ID_3/view,FILE_ID_3,deleted
```

**Status Values**:
- `available`: File is accessible and was successfully retrieved
- `deleted`: File is deleted, inaccessible, or access was denied (file ID shown as title)

### Mode 2: Conversion

Convert Google Drive documents to markdown with hierarchical organization.

```bash
./gdrive-crawler convert \
  -input enhanced-links.csv \
  -output ./docs \
  -credentials credentials.json \
  -workers 10 \
  -verbose
```

**Input CSV Format** (`enhanced-links.csv`):
```csv
link,title,tags,frag1,frag2,frag3,frag4,frag5
https://drive.google.com/file/d/FILE_ID/view,Getting Started,tutorial;beginner,guides,tutorials,,,
https://drive.google.com/file/d/FILE_ID/view,API Reference,api;advanced,reference,api,,,
```

**Output Structure**:
```
docs/
├── guides/
│   └── tutorials/
│       └── Getting Started.md
└── reference/
    └── api/
        └── API Reference.md
```

**Generated Markdown File**:
```markdown
---
description: Getting Started
editor: markdown
hash-gdrive: 2024-01-15T10:30:00.000Z
hash-content: a1b2c3d4e5f6...
published: true
tags: tutorial, beginner
title: Getting Started
---

# Getting Started

[Link to API Reference](../../reference/api/API Reference.md)
```

### CLI Flags

#### Common Flags
- `-credentials string`: Google API credentials JSON file (required)
- `-verbose`: Enable detailed logging

#### Discovery Mode Flags
- `-input string`: Input CSV file with Google Drive URLs (required)
- `-output string`: Output CSV file path (required)

#### Conversion Mode Flags
- `-input string`: Input CSV with link, title, tags, frag1-5 columns (required)
- `-output string`: Output directory path (default: `./output`)
- `-workers int`: Number of concurrent workers (default: 5)
- `-dry-run`: Preview actions without writing files

## Architecture

### Project Structure

```
webscrape-to-wikijs/
├── cmd/
│   └── gdrive-crawler/
│       └── main.go              # CLI entry point
├── internal/
│   ├── auth/
│   │   └── auth.go              # Google Drive authentication
│   ├── csv/
│   │   ├── parser.go            # CSV input parsing
│   │   └── writer.go            # CSV output writing
│   ├── discovery/
│   │   └── discovery.go         # Mode 1: Folder traversal
│   ├── conversion/
│   │   └── conversion.go        # Mode 2: Document conversion
│   └── utils/
│       ├── path.go              # Path sanitization & relative path calculation
│       └── hash.go              # Content hashing
├── go.mod
├── go.sum
└── README.md
```

### Key Components

#### Authentication (`internal/auth`)
- Supports both service account and OAuth2 credentials
- Automatic credential type detection
- Context-aware session management

#### Discovery (`internal/discovery`)
- Recursive folder traversal using Google Drive API
- Duplicate detection to avoid processing same files multiple times
- Exponential backoff for rate limit handling
- Progress logging for long-running operations
- **Deleted file tracking**: Files that are deleted or inaccessible are still indexed with status="deleted" for documentation tracking

#### Deleted File Handling
When the tool encounters a deleted or inaccessible file, it:
1. Still adds the file to the discovery output CSV
2. Sets the status to "deleted"
3. Uses the file ID as the title (since the actual name is unavailable)
4. Logs a warning message

This is useful for:
- Tracking documentation that has been removed
- Identifying broken references in your documentation
- Maintaining a complete historical record
- Auditing file deletions

#### Conversion (`internal/conversion`)
- Concurrent document processing with worker pools
- Native markdown export for Google Docs
- PDF text extraction using `github.com/ledongthuc/pdf`
- Link rewriting with relative path calculation
- YAML frontmatter generation
- Directory structure creation based on fragments

#### Link Rewriting Algorithm
1. Parse all Google Drive links in markdown content
2. Extract file IDs from URLs
3. Look up target documents in CSV inventory
4. Calculate relative paths using fragment hierarchy
5. Replace absolute URLs with relative markdown links

#### Path Calculation
```
Source: frag1=guides, frag2=tutorials, frag3=
Target: frag1=reference, frag2=api, frag3=

Relative path: ../../reference/api/target.md
```

## CSV Column Reference

### Discovery Input
- `url` or `link`: Google Drive URL (file or folder)

### Conversion Input
- `link`: Google Drive file URL (required)
- `title`: Document title (required)
- `tags`: Comma-separated tags (optional)
- `frag1` through `frag5`: Directory hierarchy fragments (optional)

### Fragments
Fragments define the output directory structure. Empty fragments are skipped.

**Example**:
```csv
link,title,frag1,frag2,frag3
...,Doc1,guides,getting-started,
...,Doc2,reference,api,authentication
```

**Output**:
```
output/
├── guides/
│   └── getting-started/
│       └── Doc1.md
└── reference/
    └── api/
        └── authentication/
            └── Doc2.md
```

## Frontmatter Fields

Generated YAML frontmatter includes:

- `description`: Document title
- `editor`: Always set to "markdown"
- `hash-gdrive`: Google Drive modification timestamp
- `hash-content`: SHA256 hash of markdown content
- `published`: Always set to true
- `tags`: Comma-separated tags from CSV
- `title`: Document title

## Error Handling

The tool handles various error scenarios gracefully:

- **Invalid Permissions**: Skips files with access errors and logs warnings
- **Network Timeouts**: Retries with exponential backoff (5 attempts)
- **Malformed CSV**: Reports line numbers and continues processing valid rows
- **Duplicate Links**: Uses first occurrence and logs warnings
- **API Rate Limits**: Automatic retry with delays (1s, 2s, 4s, 8s, 16s)

## Performance Considerations

### Concurrency
- Default worker count: 5
- Recommended for large datasets: 10-20 workers
- Google Drive API has per-minute quotas

### Memory
- Streams large files to avoid memory issues
- Temp files used for PDF processing
- Link map cached in memory for O(1) lookups

### Rate Limiting
- Built-in exponential backoff
- Respects Google Drive API quotas
- Verbose mode shows retry attempts

## Troubleshooting

### "Failed to authenticate"
- Verify credentials file path
- Check credentials file format (JSON)
- Ensure Drive API is enabled in Google Cloud Console
- For service accounts, share Drive folders with service account email

### "No 'url' or 'link' column found"
- Check CSV header row
- Ensure column name is exactly "url" or "link" (case-insensitive)
- Verify CSV is properly formatted

### "Failed to extract file ID"
- Verify Google Drive URL format
- Supported formats:
  - `https://drive.google.com/file/d/{ID}/view`
  - `https://drive.google.com/folders/{ID}`
  - `https://docs.google.com/document/d/{ID}/edit`

### "Rate limited"
- Reduce worker count with `-workers` flag
- Wait a few minutes before retrying
- Check Google Cloud Console quotas

### "Unsupported file type"
- Currently supported: Google Docs, PDFs
- Other Google Workspace types (Sheets, Slides) require export format specification

## Development

### Running Tests
```bash
go test ./...
```

### Building
```bash
go build -o gdrive-crawler ./cmd/gdrive-crawler
```

### Adding New File Type Support
1. Add MIME type handling in `internal/conversion/conversion.go`
2. Implement export/conversion function
3. Update documentation

## Roadmap

- [ ] Support for Google Sheets → Markdown tables
- [ ] Image asset downloading and local referencing
- [ ] Incremental updates (only process changed documents)
- [ ] Broken link validation
- [ ] Multi-language documentation support
- [ ] Wiki.js API integration for direct upload
- [ ] Progress bars for batch operations
- [ ] Resume support for interrupted operations

## License

MIT License

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Acknowledgments

- Google Drive API
- [ledongthuc/pdf](https://github.com/ledongthuc/pdf) for PDF text extraction
