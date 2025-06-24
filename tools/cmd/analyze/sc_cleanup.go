package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// sc_Cleanup performs cleanup operations after reconstruction
// It closes the output file, removes temporary files, and looks for hanging tmp files
func sc_Cleanup(scState *sc_State) error {
	fmt.Printf("Performing cleanup operations...\n")

	// Close the output file if it's open
	if scState.OutFile != nil {
		outputName := scState.OutFile.Name()
		fmt.Printf("Closing output file: %s\n", outputName)
		if err := scState.OutFile.Close(); err != nil {
			fmt.Printf("Warning: Error closing output file: %v\n", err)
		}
		scState.OutFile = nil
	}

	// Remove temporary files
	if scState.TempDir != "" {
		fmt.Printf("Removing temporary directory: %s\n", scState.TempDir)
		if err := os.RemoveAll(scState.TempDir); err != nil {
			fmt.Printf("Warning: Error removing temporary directory: %v\n", err)
		}
		scState.TempDir = ""
	}

	// Close all section files
	if scState.SectionFiles != nil {
		// Get all sections
		sections := scState.SectionFiles.List()
		
		// Close each section file
		for _, section := range sections {
			if section.TmpFile != nil {
				if err := section.TmpFile.Close(); err != nil {
					fmt.Printf("Warning: Error closing section file %s: %v\n", section.Type, err)
				}
			}
		}
		
		// Reset section files
		scState.SectionFiles = NewSections()
	}

	// Look for hanging temporary files from past crashed executions
	sc_CleanupHangingTempFiles()

	// Calculate and report duration if we have a start time
	if !scState.StartTime.IsZero() {
		duration := time.Since(scState.StartTime)
		fmt.Printf("Total execution time: %v\n", duration)
	}

	fmt.Printf("Cleanup completed\n")
	return nil
}

// sc_CleanupHangingTempFiles looks for and removes temporary files
// from past crashed executions of the snapshot reconstruction tool
func sc_CleanupHangingTempFiles() {
	// Get the system's temporary directory
	tempDir := os.TempDir()
	fmt.Printf("Checking for hanging temporary files in %s\n", tempDir)

	// Look for directories that match our pattern
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		fmt.Printf("Warning: Error reading temporary directory: %v\n", err)
		return
	}

	// Count how many files we remove
	removedCount := 0

	// Check each entry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if this is one of our temporary directories
		// We look for directories that start with "sc-reconstruct-" or "sc_combine_"
		name := entry.Name()
		if strings.HasPrefix(name, "sc-reconstruct-") || strings.HasPrefix(name, "sc_combine_") {
			// Check if the directory is older than 24 hours
			dirPath := filepath.Join(tempDir, name)
			info, err := os.Stat(dirPath)
			if err != nil {
				fmt.Printf("Warning: Error checking temporary directory %s: %v\n", name, err)
				continue
			}

			// If the directory is older than 24 hours, remove it
			if time.Since(info.ModTime()) > 24*time.Hour {
				fmt.Printf("Removing old temporary directory: %s (age: %v)\n",
					name, time.Since(info.ModTime()))
				if err := os.RemoveAll(dirPath); err != nil {
					fmt.Printf("Warning: Error removing temporary directory %s: %v\n", name, err)
				} else {
					removedCount++
				}
			}
		}
	}

	if removedCount > 0 {
		fmt.Printf("Removed %d old temporary directories\n", removedCount)
	} else {
		fmt.Printf("No hanging temporary files found\n")
	}
}
