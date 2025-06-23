package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// Simple version of sc_State to hold state for the analysis
type analyzeState struct {
	SnapshotPath string
	File         *os.File
	TempDir      string
	SectionFiles map[string]*os.File
}

// Cleanup closes all open files and removes temporary files
func (state *analyzeState) Cleanup() {
	// Close all section files
	for _, file := range state.SectionFiles {
		file.Close()
	}

	// Close the snapshot file if it's open
	if state.File != nil {
		state.File.Close()
		state.File = nil
	}

	// Remove the temporary directory and all its contents
	if state.TempDir != "" {
		os.RemoveAll(state.TempDir)
		state.TempDir = ""
	}
}

// Init initializes the state by opening the snapshot file
func (state *analyzeState) Init(snapshotPath string) error {
	state.SnapshotPath = snapshotPath
	state.SectionFiles = make(map[string]*os.File)

	// Open the snapshot file
	file, err := os.Open(snapshotPath)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}
	state.File = file

	// Create a temporary directory for section files
	tmpDir, err := os.MkdirTemp("", "analyze_tmp_")
	if err != nil {
		file.Close()
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	state.TempDir = tmpDir

	return nil
}

// analyzeTmpFiles command
var analyzeTmpFilesCmd = &cobra.Command{
	Use:   "analyze-tmp-files <snapshot-file>",
	Short: "Analyze temporary files created during snapshot parsing",
	Long:  `Analyze the temporary files created during snapshot parsing and print their sizes.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAnalyzeTmpFiles,
}

func runAnalyzeTmpFiles(cmd *cobra.Command, args []string) error {
	snapshotPath := args[0]

	fmt.Printf("Analyzing snapshot: %s\n", snapshotPath)

	// Create and initialize state
	state := &analyzeState{}
	err := state.Init(snapshotPath)
	if err != nil {
		return fmt.Errorf("failed to initialize state: %w", err)
	}
	defer state.Cleanup()

	// Create some dummy temporary files to simulate the parsing process
	// In a real implementation, this would be replaced by actual parsing logic
	fmt.Println("Creating dummy temporary files for analysis...")

	// Create a few dummy section files of different sizes
	dummySizes := map[string]int{
		"section_1":   1024,    // 1KB
		"section_2":   10240,   // 10KB
		"section_3":   102400,  // 100KB
		"section_7_1": 1048576, // 1MB (accounts)
		"section_7_2": 524288,  // 512KB (messages)
	}

	for name, size := range dummySizes {
		// Create a temporary file
		tmpFile, err := os.Create(filepath.Join(state.TempDir, name))
		if err != nil {
			return fmt.Errorf("failed to create temporary file: %w", err)
		}

		// Write dummy data to the file
		data := make([]byte, size)
		_, err = tmpFile.Write(data)
		if err != nil {
			tmpFile.Close()
			return fmt.Errorf("failed to write to temporary file: %w", err)
		}

		// Store the file handle
		state.SectionFiles[name] = tmpFile
	}

	// Print information about the temporary files
	fmt.Printf("\n=== TEMPORARY FILES ANALYSIS ===\n")
	fmt.Printf("Number of temporary files: %d\n", len(state.SectionFiles))

	var totalTmpFileSize int64
	for key, file := range state.SectionFiles {
		// Get file size
		stat, err := file.Stat()
		if err != nil {
			fmt.Printf("  Section file: %s (error getting size: %v)\n", key, err)
			continue
		}
		fileSize := stat.Size()
		totalTmpFileSize += fileSize
		fmt.Printf("  Section file: %s (size: %d bytes)\n", key, fileSize)
	}
	fmt.Printf("Total size of all temporary files: %d bytes\n", totalTmpFileSize)
	fmt.Printf("=== END OF TEMPORARY FILES ANALYSIS ===\n")

	return nil
}

func init() {
	// Add the command to the root command
	rootCmd.AddCommand(analyzeTmpFilesCmd)
}
