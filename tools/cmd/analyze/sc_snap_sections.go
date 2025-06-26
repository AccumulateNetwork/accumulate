// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// SectionFileInfo represents information about a section file extracted from a snapshot
type SectionFileInfo struct {
	Index    int    // Section index for ordering
	Type     uint32 // Section type
	Size     int64  // Size of section data
	FilePath string // Path to the section data file
}

// FindSectionFiles finds all section files in the temporary directory
// and returns them sorted by their section index
func FindSectionFiles(tmpDir string) ([]SectionFileInfo, error) {
	// Read all files in the temporary directory
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read temporary directory: %w", err)
	}

	// Collect section files
	var sections []SectionFileInfo
	for _, file := range files {
		// Skip non-section files
		if !strings.HasPrefix(file.Name(), "Order_") {
			continue
		}

		// Extract index and type from filename
		index, sectionType, err := extractSectionInfo(file.Name())
		if err != nil {
			// Skip files with invalid names
			continue
		}

		// Get file size
		fileInfo, err := os.Stat(filepath.Join(tmpDir, file.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to stat file %s: %w", file.Name(), err)
		}

		// Add to sections list
		sections = append(sections, SectionFileInfo{
			Index:    index,
			Type:     sectionType,
			Size:     fileInfo.Size(),
			FilePath: filepath.Join(tmpDir, file.Name()),
		})
	}

	// Sort sections by index
	sort.Slice(sections, func(i, j int) bool {
		return sections[i].Index < sections[j].Index
	})

	return sections, nil
}

// extractSectionInfo extracts the section index and type from a filename
// Example: "Order_01_Section_Type_11.bin" -> index=1, type=11
func extractSectionInfo(filename string) (int, uint32, error) {
	// Extract index
	indexRegex := regexp.MustCompile(`Order_(\d+)_`)
	indexMatches := indexRegex.FindStringSubmatch(filename)
	if len(indexMatches) < 2 {
		return 0, 0, fmt.Errorf("invalid filename format: %s", filename)
	}
	index, err := strconv.Atoi(indexMatches[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid section index in filename %s: %w", filename, err)
	}

	// Extract type
	typeRegex := regexp.MustCompile(`Section_Type_(\d+)`)
	typeMatches := typeRegex.FindStringSubmatch(filename)
	if len(typeMatches) < 2 {
		return 0, 0, fmt.Errorf("invalid filename format: %s", filename)
	}
	typeVal, err := strconv.ParseUint(typeMatches[1], 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid section type in filename %s: %w", filename, err)
	}

	return index, uint32(typeVal), nil
}

// GetNextSection returns the next section after the header
// The header is always section index 0, so we're looking for index 1
func GetNextSection(tmpDir string) (*SectionFileInfo, error) {
	sections, err := FindSectionFiles(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find section files: %w", err)
	}

	// Find the section with index 1 (the one after the header)
	for _, section := range sections {
		if section.Index == 1 {
			return &section, nil
		}
	}

	return nil, fmt.Errorf("no section found after the header")
}

// AddNextSection adds the next section after the header to the snapshot file
func AddNextSection(tmpDir string, outputPath string) error {
	// Open the output file for appending
	outputFile, err := os.OpenFile(outputPath, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open output file: %w", err)
	}
	defer outputFile.Close()
	
	// First, verify that the file has a valid SNAP header
	header := make([]byte, 8)
	_, err = outputFile.ReadAt(header, 0)
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}
	
	if string(header[:4]) != "SNAP" {
		return fmt.Errorf("invalid SNAP header in output file")
	}

	// Get the next section
	nextSection, err := GetNextSection(tmpDir)
	if err != nil {
		return fmt.Errorf("failed to get next section: %w", err)
	}

	fmt.Printf("Adding next section: Index=%d, Type=%d, Size=%d bytes, File=%s\n",
		nextSection.Index, nextSection.Type, nextSection.Size, filepath.Base(nextSection.FilePath))

	// Seek to the end of the file
	currentPos, err := outputFile.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("failed to seek to end of file: %w", err)
	}

	// Ensure we're at a 64-byte boundary
	if currentPos%64 != 0 {
		padding := make([]byte, 64-(currentPos%64))
		_, err = outputFile.Write(padding)
		if err != nil {
			return fmt.Errorf("failed to write padding: %w", err)
		}
		currentPos, err = outputFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("failed to get current position: %w", err)
		}
	}

	// Open the section file
	sectionFile, err := os.Open(nextSection.FilePath)
	if err != nil {
		return fmt.Errorf("failed to open section file: %w", err)
	}
	defer sectionFile.Close()

	// Copy the section data to the output file
	bytesWritten, err := io.Copy(outputFile, sectionFile)
	if err != nil {
		return fmt.Errorf("failed to copy section data: %w", err)
	}

	fmt.Printf("Added section to snapshot file at offset 0x%x, wrote %d bytes\n",
		currentPos, bytesWritten)

	return nil
}

// ReconstructWithNextSection creates a snapshot file with the header and the next section
func ReconstructWithNextSection(tmpDir string, outputPath string) error {
	// First, reconstruct with just the header
	err := reconstructHeaderOnly(tmpDir, outputPath)
	if err != nil {
		return fmt.Errorf("failed to reconstruct header: %w", err)
	}

	// Then add the next section
	err = AddNextSection(tmpDir, outputPath)
	if err != nil {
		return fmt.Errorf("failed to add next section: %w", err)
	}

	return nil
}
