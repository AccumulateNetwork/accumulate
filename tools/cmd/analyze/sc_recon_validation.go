package main

import (
	"fmt"
	"io"
	"os"
)

// sc_ValidateReconstructionImpl validates that the reconstructed snapshot matches the original
// This function will be called by sc_Run via the sc_ValidateReconstruction function variable
func sc_ValidateReconstructionImpl(originalPath, reconstructedPath string) (bool, error) {
	// Log validation start
	fmt.Printf("Validating reconstruction: comparing %s with %s\n", originalPath, reconstructedPath)

	// Open the original file
	originalFile, err := os.Open(originalPath)
	if err != nil {
		return false, fmt.Errorf("failed to open original file: %w", err)
	}
	defer originalFile.Close()

	// Open the reconstructed file
	reconstructedFile, err := os.Open(reconstructedPath)
	if err != nil {
		return false, fmt.Errorf("failed to open reconstructed file: %w", err)
	}
	defer reconstructedFile.Close()

	// Get the file sizes
	originalInfo, err := originalFile.Stat()
	if err != nil {
		return false, fmt.Errorf("failed to get original file info: %w", err)
	}

	reconstructedInfo, err := reconstructedFile.Stat()
	if err != nil {
		return false, fmt.Errorf("failed to get reconstructed file info: %w", err)
	}

	// Compare file sizes
	if originalInfo.Size() != reconstructedInfo.Size() {
		return false, fmt.Errorf("file sizes do not match: original=%d, reconstructed=%d",
			originalInfo.Size(), reconstructedInfo.Size())
	}

	fmt.Printf("File sizes match: %d bytes\n", originalInfo.Size())

	// Compare file contents byte by byte
	const bufferSize = 64 * 1024 // 64KB buffer
	originalBuffer := make([]byte, bufferSize)
	reconstructedBuffer := make([]byte, bufferSize)

	var position int64
	var bytesCompared int64
	var chunksCompared int

	fmt.Printf("Comparing file contents...\n")

	for {
		originalRead, err1 := originalFile.Read(originalBuffer)
		reconstructedRead, err2 := reconstructedFile.Read(reconstructedBuffer)

		// Check for read errors
		if err1 != nil && err1 != io.EOF {
			return false, fmt.Errorf("error reading original file: %w", err1)
		}
		if err2 != nil && err2 != io.EOF {
			return false, fmt.Errorf("error reading reconstructed file: %w", err2)
		}

		// Check if we've reached the end of both files
		if originalRead == 0 && reconstructedRead == 0 {
			break
		}

		// Check for size mismatch in chunks
		if originalRead != reconstructedRead {
			return false, fmt.Errorf("chunk size mismatch at position %d: original=%d bytes, reconstructed=%d bytes",
				position, originalRead, reconstructedRead)
		}

		// Compare the bytes read
		for i := 0; i < originalRead; i++ {
			if originalBuffer[i] != reconstructedBuffer[i] {
				// Found a mismatch, report the position and context
				mismatchPos := position + int64(i)

				// Get some context around the mismatch
				contextStart := sc_max(0, i-16)
				contextEnd := sc_min(i+16, originalRead)

				return false, fmt.Errorf("mismatch at byte position %d: original=0x%02x, reconstructed=0x%02x\n"+
					"Original context: %x\nReconstructed context: %x",
					mismatchPos, originalBuffer[i], reconstructedBuffer[i],
					originalBuffer[contextStart:contextEnd],
					reconstructedBuffer[contextStart:contextEnd])
			}
		}

		// Update position and statistics
		position += int64(originalRead)
		bytesCompared += int64(originalRead)
		chunksCompared++

		// Print progress for large files
		if chunksCompared%100 == 0 {
			fmt.Printf("  Compared %d MB so far...\n", bytesCompared/(1024*1024))
		}

		// Check if we've reached the end of either file
		if err1 == io.EOF || err2 == io.EOF {
			break
		}
	}

	fmt.Printf("Validation successful: files match byte-for-byte (%d bytes compared)\n", bytesCompared)
	return true, nil
}

// Using sc_min and sc_max from sc_utils.go instead
// to avoid redeclaration issues

// Using sc_sortSectionKeys from sc_utils.go instead
// to avoid redeclaration issues

// RunTests runs all unit tests for the reconstruction components
func sc_RunReconstructionTests() error {
	fmt.Printf("Running reconstruction unit tests...\n\n")

	// Test sc_StartReconstruction
	err := sc_StartReconstructionTest()
	if err != nil {
		return fmt.Errorf("sc_StartReconstructionTest failed: %w", err)
	}

	// Test sc_WriteSection
	err = sc_WriteSectionTest()
	if err != nil {
		return fmt.Errorf("sc_WriteSectionTest failed: %w", err)
	}

	// Test sc_UpdateOffsets
	err = sc_UpdateOffsetsTest()
	if err != nil {
		return fmt.Errorf("sc_UpdateOffsetsTest failed: %w", err)
	}

	// Test sc_ReconstructionReport
	err = sc_ReconstructionReportTest()
	if err != nil {
		return fmt.Errorf("sc_ReconstructionReportTest failed: %w", err)
	}

	fmt.Printf("\nAll reconstruction tests passed!\n")
	return nil
}
