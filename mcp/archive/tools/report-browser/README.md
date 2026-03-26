# MCP Report Browser

A web-based browser for viewing test reports, coverage analysis, and development summaries for the Accumulate MCP project.

## Quick Start

```bash
# Start the report browser
./start-report-browser.sh

# Or manually
go build -o report-browser .
./report-browser --port 8080 --dir ../../
```

Then open your browser to: **http://localhost:8080**

## Features

- **📊 Report Dashboard**: Browse all test reports, summaries, and coverage files
- **🔍 Search**: Filter reports by name or path
- **🏷️ Categories**: Filter by test, coverage, summary, or integration reports
- **📝 Markdown Rendering**: Beautiful GitHub-style rendering of .md files
- **⚡ Real-time**: Auto-discovers new reports in the directory

## Usage

### Start Server

```bash
./start-report-browser.sh
```

The server will:
1. Build the binary if needed
2. Start on `http://localhost:8080`
3. Scan the `mcp/` directory for reports
4. Log output to `report-browser.log`

### Command Line Options

```bash
./report-browser [options]

Options:
  --port string   Port to run server on (default "8080")
  --dir string    Directory containing reports (default "../../")
```

### Stopping the Server

```bash
killall report-browser
```

## Report Discovery

The browser automatically discovers files that:
- Have `.md` extension, OR
- Contain "report", "summary", "results", "coverage", or "test" in the path

### Included Reports

- Test coverage reports (`test-coverage-report.md`)
- Integration test results (`integration-test-results.md`)
- Validation summaries (`VALIDATION_SUMMARY.md`)
- Database health reports (`database_health_report.md`)
- Phase summaries (`phase1_summary.md`, etc.)
- And many more...

## Development

### Requirements

- Go 1.19 or later
- `github.com/gomarkdown/markdown` package

### Building

```bash
go build -o report-browser .
```

### Project Structure

```
report-browser/
├── main.go              # Main server code
├── templates/
│   └── index.html       # Dashboard template
├── report-browser       # Compiled binary
├── start-report-browser.sh  # Startup script
└── README.md           # This file
```

## Features Details

### Search

Type in the search box to filter reports by:
- File name
- Full path
- Any text in the display

### Category Filters

Click category buttons to show only:
- **All**: Show all reports
- **Tests**: Test-related files
- **Coverage**: Coverage analysis
- **Summaries**: Summary documents
- **Integration**: Integration tests

### Markdown Rendering

Markdown files are rendered with:
- GitHub-style dark theme
- Syntax-highlighted code blocks
- Tables, lists, and blockquotes
- Heading anchors
- External link handling

## Troubleshooting

### Port Already in Use

If port 8080 is already in use:

```bash
# Kill existing server
killall report-browser

# Or use a different port
./report-browser --port 8081
```

### No Reports Found

Check that you're running from the correct directory:

```bash
cd mcp/tools/report-browser
./start-report-browser.sh
```

### Build Errors

Make sure dependencies are installed:

```bash
go mod tidy
go build -o report-browser .
```

## Future Enhancements

Potential improvements:
- [ ] Live reload when reports change
- [ ] Export filtered reports
- [ ] Compare reports side-by-side
- [ ] Report history/versioning
- [ ] API endpoint for programmatic access
- [ ] Custom report templates

## License

Same as the Accumulate project (MIT)
