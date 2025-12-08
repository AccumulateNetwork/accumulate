package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

//go:embed templates/*
var templateFS embed.FS

type Report struct {
	Name         string
	Path         string
	ModTime      time.Time
	Size         int64
	IsDir        bool
	RelativePath string
}

var (
	port       = flag.String("port", "8080", "Port to run server on")
	reportDir  = flag.String("dir", "../../", "Directory containing reports")
	serverAddr string
)

func main() {
	flag.Parse()

	serverAddr = "http://localhost:" + *port

	absReportDir, err := filepath.Abs(*reportDir)
	if err != nil {
		log.Fatalf("Failed to get absolute path: %v", err)
	}
	*reportDir = absReportDir

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/reports/", handleReport)
	http.HandleFunc("/reports", handleReportRedirect)

	log.Printf("Report Browser starting on http://localhost:%s", *port)
	log.Printf("Serving reports from: %s", *reportDir)
	log.Fatalf("Server error: %v", http.ListenAndServe(":"+*port, nil))
}

func handleReportRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusMovedPermanently)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	reports, err := scanReports(*reportDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error scanning reports: %v", err), http.StatusInternalServerError)
		return
	}

	tmplContent, err := templateFS.ReadFile("templates/index.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading template: %v", err), http.StatusInternalServerError)
		return
	}

	funcMap := template.FuncMap{
		"getCategoryFromName": getCategoryFromName,
		"getCategoryLabel":    getCategoryLabel,
		"formatSize":          formatSize,
	}

	tmpl, err := template.New("index").Funcs(funcMap).Parse(string(tmplContent))
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing template: %v", err), http.StatusInternalServerError)
		return
	}

	data := struct {
		Reports []Report
		BaseDir string
	}{
		Reports: reports,
		BaseDir: *reportDir,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Error executing template: %v", err)
	}
}

func handleReport(w http.ResponseWriter, r *http.Request) {
	reportPath := strings.TrimPrefix(r.URL.Path, "/reports/")
	fullPath := filepath.Join(*reportDir, reportPath)

	// Security check: ensure path is within reportDir
	absPath, err := filepath.Abs(fullPath)
	if err != nil || !strings.HasPrefix(absPath, *reportDir) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading file: %v", err), http.StatusNotFound)
		return
	}

	// Check if it's a markdown file
	if strings.HasSuffix(fullPath, ".md") {
		renderMarkdown(w, content, reportPath)
		return
	}

	// Serve raw file for non-markdown
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(content)
}

func renderMarkdown(w http.ResponseWriter, content []byte, title string) {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.Footnotes
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(content)

	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)
	htmlContent := markdown.Render(doc, renderer)

	tmplContent := `<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>{{.Title}}</title>
	<style>
		body {
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
			line-height: 1.6;
			max-width: 900px;
			margin: 0 auto;
			padding: 20px;
			background: #0d1117;
			color: #c9d1d9;
		}
		h1, h2, h3, h4, h5, h6 {
			color: #58a6ff;
			border-bottom: 1px solid #21262d;
			padding-bottom: 0.3em;
		}
		a {
			color: #58a6ff;
			text-decoration: none;
		}
		a:hover {
			text-decoration: underline;
		}
		code {
			background: #161b22;
			padding: 0.2em 0.4em;
			border-radius: 3px;
			font-family: 'Monaco', 'Consolas', monospace;
			color: #79c0ff;
		}
		pre {
			background: #161b22;
			padding: 16px;
			overflow: auto;
			border-radius: 6px;
			border: 1px solid #30363d;
		}
		pre code {
			background: none;
			padding: 0;
			color: #c9d1d9;
		}
		table {
			border-collapse: collapse;
			width: 100%;
			margin: 1em 0;
		}
		th, td {
			border: 1px solid #30363d;
			padding: 8px 12px;
			text-align: left;
		}
		th {
			background: #161b22;
			font-weight: 600;
		}
		blockquote {
			border-left: 4px solid #30363d;
			padding-left: 1em;
			color: #8b949e;
			margin: 1em 0;
		}
		.back-link {
			display: inline-block;
			margin-bottom: 20px;
			padding: 8px 16px;
			background: #21262d;
			border-radius: 6px;
		}
	</style>
</head>
<body>
	<div class="back-link">
		<a href="/">← Back to Report List</a>
	</div>
	<div class="markdown-content">
		{{.Content}}
	</div>
</body>
</html>`

	tmpl, err := template.New("markdown").Parse(tmplContent)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	data := struct {
		Title   string
		Content template.HTML
	}{
		Title:   title,
		Content: template.HTML(htmlContent),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, data)
}

func scanReports(dir string) ([]Report, error) {
	var reports []Report

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and files
		if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip certain directories
		skipDirs := []string{"node_modules", "vendor", ".git", "bin", "build"}
		if info.IsDir() {
			for _, skip := range skipDirs {
				if info.Name() == skip {
					return filepath.SkipDir
				}
			}
		}

		// Only include markdown files and relevant reports
		if !info.IsDir() && (strings.HasSuffix(path, ".md") ||
			strings.Contains(path, "report") ||
			strings.Contains(path, "summary") ||
			strings.Contains(path, "results") ||
			strings.Contains(path, "coverage") ||
			strings.Contains(path, "test")) {

			relPath, _ := filepath.Rel(dir, path)
			reports = append(reports, Report{
				Name:         info.Name(),
				Path:         path,
				RelativePath: relPath,
				ModTime:      info.ModTime(),
				Size:         info.Size(),
				IsDir:        info.IsDir(),
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by modification time (newest first)
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].ModTime.After(reports[j].ModTime)
	})

	return reports, nil
}

func getCategoryFromName(name, path string) string {
	nameLower := strings.ToLower(name)
	pathLower := strings.ToLower(path)

	if strings.Contains(nameLower, "test") || strings.Contains(pathLower, "test") {
		return "test"
	}
	if strings.Contains(nameLower, "coverage") || strings.Contains(pathLower, "coverage") {
		return "coverage"
	}
	if strings.Contains(nameLower, "summary") || strings.Contains(pathLower, "summary") {
		return "summary"
	}
	if strings.Contains(nameLower, "integration") || strings.Contains(pathLower, "integration") {
		return "integration"
	}
	return "report"
}

func getCategoryLabel(name, path string) string {
	category := getCategoryFromName(name, path)
	return strings.Title(category)
}

func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.2f MB", float64(size)/(1024*1024))
}
