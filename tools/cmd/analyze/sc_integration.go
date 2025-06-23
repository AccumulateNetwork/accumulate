package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// sc_ReconstructSnapshotCmd is the main entry point for the snapshot reconstruction command
// This function will be called from sc.go
func sc_ReconstructSnapshotCmd(scState *sc_State) error {
	// Initialize the scState if needed
	if scState.SectionFiles == nil {
		scState.SectionFiles = make(map[string]*os.File)
	}

	// Determine the output path - use a default if not specified
	outputPath := "reconstructed.snap"

	// Perform the reconstruction
	return sc_reconstruct(scState, outputPath)
}

// Helper function to read a section from a file
func readSectionFromFile(file *os.File, offset int64, size int64) ([]byte, error) {
	// Seek to the section start
	_, err := file.Seek(offset, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("failed to seek to section: %w", err)
	}

	// Read the section data
	data := make([]byte, size)
	_, err = io.ReadFull(file, data)
	if err != nil {
		return nil, fmt.Errorf("failed to read section data: %w", err)
	}

	return data, nil
}

// Helper function to get a unique temporary file name
func getTempFileName(dir, prefix string) string {
	return filepath.Join(dir, fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano()))
}

// Helper function to ensure a directory exists
func ensureDirectoryExists(path string) error {
	return os.MkdirAll(path, 0755)
}

// Helper function to close a file and check for errors
func closeFile(file *os.File) error {
	if file == nil {
		return nil
	}
	return file.Close()
}

// Helper function to copy a file
func copyFile(src, dst string) error {
	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	// Copy the contents
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Sync to ensure data is written
	err = dstFile.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	return nil
}

// Helper function to get file size
func getFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// Helper function to check if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Helper function to read entire file content
func readFileContent(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Helper function to write content to a file
func writeFileContent(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

// Helper function to get a list of files in a directory
func listFilesInDir(dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var filePaths []string
	for _, file := range files {
		if !file.IsDir() {
			filePaths = append(filePaths, filepath.Join(dir, file.Name()))
		}
	}

	return filePaths, nil
}

// Helper function to get a formatted timestamp
func getTimestamp() string {
	return time.Now().Format("2006-01-02_15-04-05")
}

// Helper function to create a backup of a file
func backupFile(path string) (string, error) {
	if !fileExists(path) {
		return "", nil
	}

	backupPath := path + "." + getTimestamp() + ".bak"
	err := copyFile(path, backupPath)
	if err != nil {
		return "", err
	}

	return backupPath, nil
}

// Helper function to clean up temporary files
func cleanupTempFiles(files map[string]*os.File) {
	for _, file := range files {
		if file != nil {
			file.Close()
			os.Remove(file.Name())
		}
	}
}

// Helper function to format bytes as human-readable size
func formatByteSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// Helper function to truncate a string if it's too long
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// Helper function to join strings with a separator, skipping empty strings
func joinNonEmpty(sep string, parts ...string) string {
	var nonEmpty []string
	for _, part := range parts {
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	return strings.Join(nonEmpty, sep)
}

// Helper function to get a progress indicator string
func getProgressIndicator(current, total int) string {
	const width = 20
	progress := float64(current) / float64(total)
	filled := int(progress * float64(width))

	bar := "["
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "="
		} else {
			bar += " "
		}
	}
	bar += "]"

	return fmt.Sprintf("%s %d/%d (%.1f%%)", bar, current, total, progress*100)
}
