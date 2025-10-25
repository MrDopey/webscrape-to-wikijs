# Quick Start Guide

This guide will help you get started with the Google Drive Documentation Crawler in 5 minutes.

## Prerequisites

1. Go 1.21+ installed
2. A Google Cloud Project
3. Google Drive API enabled
4. Credentials file (service account or OAuth2)

## Installation

```bash
# Clone the repository
git clone https://github.com/yourusername/webscrape-to-wikijs.git
cd webscrape-to-wikijs

# Build the binary
go build -o gdrive-crawler ./cmd/gdrive-crawler

# Optional: Install to PATH
sudo mv gdrive-crawler /usr/local/bin/
```

## Quick Setup

### 1. Get Google Drive Credentials

**For Service Account (Recommended):**

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create/select a project
3. Enable Google Drive API
4. Create Service Account
5. Download JSON credentials
6. Share your Drive folders with the service account email

**For OAuth2:**

1. Create OAuth2 credentials in Google Cloud Console
2. Download client credentials JSON
3. The tool will prompt for authorization on first run

### 2. Create Input CSV

**For Discovery Mode** (`folders.csv`):
```csv
url
https://drive.google.com/drive/folders/YOUR_FOLDER_ID
```

**For Conversion Mode** (`enhanced-links.csv`):
```csv
link,title,tags,frag1,frag2,frag3,frag4,frag5
https://drive.google.com/file/d/FILE_ID_1/view,Getting Started,tutorial,docs,guides,,,
https://drive.google.com/file/d/FILE_ID_2/view,API Reference,api,docs,reference,,,
```

## Usage Examples

### Example 1: Discover Files

```bash
# Discover all files in your Google Drive folders
./gdrive-crawler discover \
  -input examples/folders.csv \
  -output discovered-links.csv \
  -credentials credentials.json \
  -verbose
```

**Output:** `discovered-links.csv` with all found files and their links

### Example 2: Convert to Markdown

```bash
# Convert discovered files to markdown
./gdrive-crawler convert \
  -input examples/enhanced-links.csv \
  -output ./docs \
  -credentials credentials.json \
  -workers 10 \
  -verbose
```

**Output:** Hierarchical markdown files in `./docs/`

### Example 3: Dry Run

```bash
# Preview what would be converted without writing files
./gdrive-crawler convert \
  -input enhanced-links.csv \
  -output ./docs \
  -credentials credentials.json \
  -dry-run
```

## Common Workflows

### Workflow 1: Complete Documentation Migration

```bash
# Step 1: Create folders.csv with your Drive folders
echo "url" > folders.csv
echo "https://drive.google.com/drive/folders/YOUR_FOLDER_ID" >> folders.csv

# Step 2: Discover all files
./gdrive-crawler discover \
  -input folders.csv \
  -output discovered.csv \
  -credentials credentials.json

# Step 3: Manually enhance discovered.csv with tags and fragments
# Add columns: tags, frag1, frag2, frag3, frag4, frag5
# Save as enhanced.csv

# Step 4: Convert to markdown
./gdrive-crawler convert \
  -input enhanced.csv \
  -output ./documentation \
  -credentials credentials.json \
  -workers 10
```

### Workflow 2: Incremental Updates

```bash
# Discover new/changed files
./gdrive-crawler discover \
  -input folders.csv \
  -output latest-links.csv \
  -credentials credentials.json

# Compare with previous run
# Add new entries to your enhanced CSV

# Convert only new files
./gdrive-crawler convert \
  -input new-files.csv \
  -output ./documentation \
  -credentials credentials.json
```

## Understanding the Output

### Directory Structure

With this CSV:
```csv
link,title,frag1,frag2,frag3
...,Quick Start,docs,guides,
...,API Endpoints,docs,reference,api
```

You get:
```
documentation/
├── docs/
│   ├── guides/
│   │   └── Quick Start.md
│   └── reference/
│       └── api/
│           └── API Endpoints.md
```

### Markdown File Format

```markdown
---
description: Quick Start
editor: markdown
hash-gdrive: 2024-01-15T10:30:00.000Z
hash-content: abc123...
published: true
tags: tutorial, guide
title: Quick Start
---

# Quick Start

Your document content here...

[Link to API Endpoints](../reference/api/API Endpoints.md)
```

## Troubleshooting

### "Failed to authenticate"
- Check credentials file path
- Verify Drive API is enabled
- For service accounts, share folders with the service account email

### "No 'url' or 'link' column found"
- Check CSV has correct header row
- Column name must be exactly "url" or "link" (case-insensitive)

### "Rate limited"
- Reduce worker count: `-workers 3`
- Wait a few minutes between runs
- Check Google Cloud Console quotas

### "Unsupported file type"
- Currently supports: Google Docs, PDFs
- Other types will be skipped with a warning

## Next Steps

1. Read the full [README.md](README.md) for detailed documentation
2. Check the [examples/](examples/) directory for sample CSV files
3. Run tests: `go test ./...`
4. Customize fragment structure for your organization

## Tips

- **Use fragments wisely**: Think about your documentation hierarchy
- **Tag consistently**: Use the same tag format across all documents
- **Start small**: Test with a few documents first
- **Enable verbose mode**: Use `-verbose` to see what's happening
- **Dry run first**: Use `-dry-run` to preview output
- **Adjust workers**: More workers = faster but more API calls

## Support

- File issues: [GitHub Issues](https://github.com/yourusername/webscrape-to-wikijs/issues)
- Read the docs: [Full Documentation](README.md)
- Check examples: [examples/](examples/)
