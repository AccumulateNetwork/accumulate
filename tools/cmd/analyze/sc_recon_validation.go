package main

import (
	"fmt"
	"io"
	"os"
)

// formatByteSize formats a byte size into a human-readable string (KB, MB, GB)
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

// sc_ValidateReconstructionImpl validates that the reconstructed snapshot matches the original
// This function will be called by sc_Run via the sc_ValidateReconstruction function variable
func sc_ValidateReconstructionImpl(originalPath, reconstructedPath string, scState *sc_State) (bool, error) {
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

	// No need to check section header alignment anymore

	// Get the file sizes
	originalInfo, err := originalFile.Stat()
	if err != nil {
		return false, fmt.Errorf("failed to get original file info: %w", err)
	}

	reconstructedInfo, err := reconstructedFile.Stat()
	if err != nil {
		return false, fmt.Errorf("failed to get reconstructed file info: %w", err)
	}

	// Print detailed size information
	fmt.Printf("Original file size:      %d bytes (%s)\n", originalInfo.Size(), formatByteSize(originalInfo.Size()))
	fmt.Printf("Reconstructed file size: %d bytes (%s)\n", reconstructedInfo.Size(), formatByteSize(reconstructedInfo.Size()))

	// Compare file sizes
	fileSizesMatch := originalInfo.Size() == reconstructedInfo.Size()
	if !fileSizesMatch {
		fmt.Printf("Warning: File sizes do not match: original=%d, reconstructed=%d\n", 
			originalInfo.Size(), reconstructedInfo.Size())
		fmt.Printf("Continuing with byte-by-byte comparison to find the first difference...\n")
	} else {
		fmt.Printf("File sizes match: %d bytes\n", originalInfo.Size())
	}

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

				// Check if this is at a 64-byte boundary (potential section header start)
				if mismatchPos%64 == 0 {
					fmt.Printf("NOTE: Mismatch occurs at a 64-byte boundary (potential section header start)\n")
				}

				// Check if this is within the first 16 bytes of a 64-byte boundary (section header data)
				if mismatchPos%64 < 16 {
					fmt.Printf("NOTE: Mismatch occurs within a section header (first 16 bytes of a 64-byte boundary)\n")
				}

				// Get window size for context - use a smaller window for better readability
				const windowSize = 256 // 256 byte window
				windowStart := mismatchPos - windowSize/2
				if windowStart < 0 {
					windowStart = 0
				}

				// Read windows of data around the mismatch for detailed comparison
				originalWindow := make([]byte, windowSize)
				reconstructedWindow := make([]byte, windowSize)

				// Save current position
				currentPos, _ := originalFile.Seek(0, io.SeekCurrent)

				// Seek to window start
				_, err := originalFile.Seek(windowStart, io.SeekStart)
				if err != nil {
					return false, fmt.Errorf("failed to seek in original file: %w", err)
				}

				// Read window of data
				originalRead, err := io.ReadFull(originalFile, originalWindow)
				if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
					return false, fmt.Errorf("failed to read from original file: %w", err)
				}

				// Seek to window start in reconstructed file
				_, err = reconstructedFile.Seek(windowStart, io.SeekStart)
				if err != nil {
					return false, fmt.Errorf("failed to seek in reconstructed file: %w", err)
				}

				// Read window of data
				reconstructedRead, err := io.ReadFull(reconstructedFile, reconstructedWindow)
				if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
					return false, fmt.Errorf("failed to read from reconstructed file: %w", err)
				}

				// Print hex dump of the difference
				fmt.Printf("\nMismatch found at position %d (0x%X)\n", mismatchPos, mismatchPos)
				fmt.Printf("Original byte: 0x%02X, Reconstructed byte: 0x%02X\n\n", originalBuffer[i], reconstructedBuffer[i])

				// Restore original position
				_, err = originalFile.Seek(currentPos, io.SeekStart)
				if err != nil {
					return false, fmt.Errorf("failed to restore position in original file: %w", err)
				}
				_, err = reconstructedFile.Seek(currentPos, io.SeekStart)
				if err != nil {
					return false, fmt.Errorf("failed to restore position in reconstructed file: %w", err)
				}

				// Calculate the relative position of the mismatch within the window
				relativeMismatchPos := int(mismatchPos - windowStart)

				// Print hex dump of original file with section headers colored
				fmt.Printf("Mismatch at offset %d (0x%X): original=0x%02X, reconstructed=0x%02X\n",
					mismatchPos, mismatchPos, originalBuffer[i], reconstructedBuffer[i])

				// Print hex dumps with mismatch highlighted
				fmt.Println("Original file:")
				for i := 0; i < originalRead; i += 16 {
					// Print offset
					fmt.Printf("%08X | ", windowStart+int64(i))

					// Print hex bytes
					for j := 0; j < 16; j++ {
						if i+j < originalRead {
							// Highlight the mismatch byte
							if i+j == relativeMismatchPos {
								fmt.Printf("\033[41m%02X\033[0m ", originalWindow[i+j]) // Red background
							} else {
								fmt.Printf("%02X ", originalWindow[i+j])
							}
						} else {
							fmt.Print("   ")
						}

						// Add extra space in the middle
						if j == 7 {
							fmt.Print(" ")
						}
					}

					// Print ASCII representation
					fmt.Print(" | ")
					for j := 0; j < 16; j++ {
						if i+j < originalRead {
							b := originalWindow[i+j]
							if b >= 32 && b <= 126 { // Printable ASCII
								fmt.Printf("%c", b)
							} else {
								fmt.Print(".")
							}
						}
					}
					fmt.Println()
				}

				fmt.Println("Reconstructed file:")
				for i := 0; i < reconstructedRead; i += 16 {
					// Print offset
					fmt.Printf("%08X | ", windowStart+int64(i))

					// Print hex bytes
					for j := 0; j < 16; j++ {
						if i+j < reconstructedRead {
							// Highlight the mismatch byte
							if i+j == relativeMismatchPos {
								fmt.Printf("\033[41m%02X\033[0m ", reconstructedWindow[i+j]) // Red background
							} else {
								fmt.Printf("%02X ", reconstructedWindow[i+j])
							}
						} else {
							fmt.Print("   ")
						}

						// Add extra space in the middle
						if j == 7 {
							fmt.Print(" ")
						}
					}

					// Print ASCII representation
					fmt.Print(" | ")
					for j := 0; j < 16; j++ {
						if i+j < reconstructedRead {
							b := reconstructedWindow[i+j]
							if b >= 32 && b <= 126 { // Printable ASCII
								fmt.Printf("%c", b)
							} else {
								fmt.Print(".")
							}
						}
					}
					fmt.Println()
				}

				return false, fmt.Errorf("reconstruction validation failed: files do not match")
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

	if !fileSizesMatch {
		return false, fmt.Errorf("file sizes do not match: original=%d, reconstructed=%d",
			originalInfo.Size(), reconstructedInfo.Size())
	}

	fmt.Printf("Validation successful: files match byte-for-byte (%d bytes compared)\n", bytesCompared)
	return true, nil
}
