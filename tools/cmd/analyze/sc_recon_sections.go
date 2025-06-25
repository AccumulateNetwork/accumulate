package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

// sc_WriteSections writes all sections to the destination file
func sc_WriteSections(scState *sc_State) error {
	fmt.Printf("Writing sections to output file...\n")

	// Get the current file position (should be after the 64-byte file header)
	_, err := scState.OutFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to get current file position: %w", err)
	}

	// Get all sections
	sections := scState.SectionFiles.List()
	if len(sections) == 0 {
		return fmt.Errorf("no sections to write")
	}

	// Check if we have input files
	if len(scState.InputFiles) == 0 {
		return fmt.Errorf("no input snapshot files available")
	}

	// Use the first input snapshot file as the source for the header and section 1_1
	firstSnapshotFile := scState.InputFiles[0]

	// Initialize output position counter
	var currentPos int64 = 0

	// Step 1: Use snapshot.Open to read the snapshot file
	// This properly handles the file header and section headers
	_, err = firstSnapshotFile.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to beginning of first snapshot file: %w", err)
	}

	// Open the snapshot using the snapshot package
	snapshotReader, err := snapshot.Open(firstSnapshotFile)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}

	// Get the snapshot header
	header := snapshotReader.Header
	fmt.Printf("Snapshot version: %d\n", header.Version)
	fmt.Printf("Root hash: %x\n", header.RootHash)

	// Seek back to the beginning of the file to copy the raw header bytes
	_, err = firstSnapshotFile.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to beginning of first snapshot file: %w", err)
	}

	// Read the 64-byte snapshot header
	headerBuf := make([]byte, 64)
	_, err = io.ReadFull(firstSnapshotFile, headerBuf)
	if err != nil {
		return fmt.Errorf("failed to read snapshot header from first snapshot file: %w", err)
	}

	// Write the header to the output file
	_, err = scState.OutFile.Write(headerBuf)
	if err != nil {
		return fmt.Errorf("failed to write snapshot header to output file: %w", err)
	}

	fmt.Printf("Copied 64-byte snapshot header from first input file\n")
	currentPos = 64

	// Step 2: Copy the header section data directly from the original file
	// Since the header section is special and part of the file header structure, we'll copy it directly
	// from the original file to ensure exact format compatibility

	// Seek to position 64 in the first input file (right after the file header)
	_, err = firstSnapshotFile.Seek(64, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to header section data in first snapshot file: %w", err)
	}

	// Find out how much data we need to read until the next 64-byte boundary
	// The header section data is always followed by the first regular section at a 64-byte boundary
	// First, find the first section after the header section in the original file
	var nextSectionOffset int64 = 0
	for _, s := range snapshotReader.Sections {
		if s.Type() != snapshot.SectionTypeHeader {
			nextSectionOffset = s.Offset()
			break
		}
	}

	if nextSectionOffset == 0 || nextSectionOffset <= 64 {
		return fmt.Errorf("could not find next section after header section in original file")
	}

	// Calculate how many bytes to read (from position 64 to the next section)
	headerDataSize := nextSectionOffset - 64
	fmt.Printf("Header section data size from original file: %d bytes\n", headerDataSize)

	// Read the header section data from the original file
	headerData := make([]byte, headerDataSize)
	_, err = io.ReadFull(firstSnapshotFile, headerData)
	if err != nil {
		return fmt.Errorf("failed to read header section data from original file: %w", err)
	}

	// Write the header section data to the output file
	_, err = scState.OutFile.Write(headerData)
	if err != nil {
		return fmt.Errorf("failed to write header section data to output file: %w", err)
	}

	currentPos += headerDataSize
	fmt.Printf("Copied header section data directly from original file, size: %d bytes\n", headerDataSize)

	// Update the section's offset in the Sections list
	if headerSection := scState.SectionFiles.Get("1_1"); headerSection != nil {
		scState.SectionFiles.UpdateOffset("1_1", 64) // The header section data starts at offset 64
	}

	// No need to add padding as we copied the exact bytes including any padding from the original file

	// Step 3: Process remaining sections
	for _, section := range sections {
		// Skip the header section (already processed as part of the file header)
		if section.Type == "1_1" {
			fmt.Printf("Skipping header section (1_1), already processed\n")
			continue
		}

		// Special handling for BPT section (section 11) - use snapshot reader like we did for header
		if strings.HasPrefix(section.Type, "11_") {
			fmt.Printf("Using snapshot reader for BPT section %s\n", section.Type)

			// For the BPT section, we need to copy the data directly from the original file
			// without writing a section header, as the original file structure has the BPT data
			// immediately following the header section data
			
			// Find the BPT section in the snapshot
			var bptSectionReader io.Reader
			var bptSectionSize int64
			for i, s := range snapshotReader.Sections {
				if s.Type() == snapshot.SectionTypeBPT {
					// Open the BPT section
					reader, err := snapshotReader.Open(snapshot.SectionTypeBPT)
					if err != nil {
						return fmt.Errorf("failed to open BPT section: %w", err)
					}
					bptSectionReader = reader
					bptSectionSize = s.Size()
					fmt.Printf("Found BPT section at index %d, size: %d bytes\n", i, bptSectionSize)
					break
				}
			}

			if bptSectionReader == nil {
				return fmt.Errorf("BPT section not found in snapshot")
			}

			// Get the size of the BPT section from our temporary file
			tmpFileInfo, err := section.TmpFile.Stat()
			if err != nil {
				return fmt.Errorf("failed to get section %s file info: %w", section.Type, err)
			}
			tmpFileSize := tmpFileInfo.Size()

			// Use the exact size from the original BPT section
			if tmpFileSize != bptSectionSize {
				fmt.Printf("Warning: BPT section size mismatch. Using original size: %d bytes, Temporary file size: %d bytes\n",
					bptSectionSize, tmpFileSize)
				tmpFileSize = bptSectionSize
			}

			// Skip writing a section header for BPT section - in the original file,
			// the BPT data follows directly after the header section data
			fmt.Printf("Writing BPT section data directly without a section header\n")

			// Seek to the beginning of the temporary file
			_, err = section.TmpFile.Seek(0, io.SeekStart)
			if err != nil {
				return fmt.Errorf("failed to seek to beginning of temporary file for %s: %w", section.Type, err)
			}

			// Read the section data from the temporary file
			sectionData := make([]byte, tmpFileSize)
			_, err = io.ReadFull(section.TmpFile, sectionData)
			if err != nil {
				return fmt.Errorf("failed to read section data for %s: %w", section.Type, err)
			}

			// Write the section data to the output file
			bytesWritten, err := scState.OutFile.Write(sectionData)
			if err != nil {
				return fmt.Errorf("failed to write section data for %s: %w", section.Type, err)
			}

			// Get current file position after writing the section data
			_, err = scState.OutFile.Seek(0, io.SeekCurrent)
			if err != nil {
				return fmt.Errorf("failed to get current file position after writing BPT section data: %w", err)
			}

			// Check if the correct number of bytes was written
			if int64(bytesWritten) != tmpFileSize {
				return fmt.Errorf("incorrect number of bytes written for %s: expected %d, got %d",
					section.Type, tmpFileSize, bytesWritten)
			}

			fmt.Printf("Successfully wrote %d bytes of data for BPT section %s\n", bytesWritten, section.Type)

			// Get current file position to check alignment
			currentPos, err = scState.OutFile.Seek(0, io.SeekCurrent)
			if err != nil {
				return fmt.Errorf("failed to get current file position for padding calculation: %w", err)
			}

			// Add padding to align to the next 64-byte boundary if needed
			if currentPos%64 != 0 {
				paddingSize := 64 - (currentPos % 64)
				padding := make([]byte, paddingSize)

				// Write padding
				_, err = scState.OutFile.Write(padding)
				if err != nil {
					return fmt.Errorf("failed to write alignment padding after BPT section %s: %w", section.Type, err)
				}

				// Update position after writing padding
				_, err = scState.OutFile.Seek(0, io.SeekCurrent)
				if err != nil {
					return fmt.Errorf("failed to get current file position after padding: %w", err)
				}

				fmt.Printf("Added %d bytes of padding after BPT section %s to align to 64-byte boundary\n",
					paddingSize, section.Type)
			}

			// Skip the normal section processing
			continue
		}

		// Get current file position to check alignment
		currentPos, err = scState.OutFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("failed to get current file position for padding calculation: %w", err)
		}

		// Ensure we're at a 64-byte boundary for each section
		if currentPos%64 != 0 {
			paddingSize := 64 - (currentPos % 64)
			padding := make([]byte, paddingSize)

			// Write padding
			_, err = scState.OutFile.Write(padding)
			if err != nil {
				return fmt.Errorf("failed to write alignment padding: %w", err)
			}

			// Update position after writing padding
			currentPos, err = scState.OutFile.Seek(0, io.SeekCurrent)
			if err != nil {
				return fmt.Errorf("failed to get current file position after padding: %w", err)
			}

			fmt.Printf("Added %d bytes of padding to align to 64-byte boundary\n", paddingSize)
		}

		// Get section type as integer using the snapshot package's section types
		sectionType, err := parseSectionType(section.Type)
		if err != nil {
			return fmt.Errorf("failed to parse section type %s: %w", section.Type, err)
		}

		// Get the size of the section data
		tmpFileInfo, err := section.TmpFile.Stat()
		if err != nil {
			return fmt.Errorf("failed to get section file info for %s: %w", section.Type, err)
		}
		tmpFileSize := tmpFileInfo.Size()

		// Validate section size
		if tmpFileSize <= 0 {
			return fmt.Errorf("invalid section size for %s: %d bytes", section.Type, tmpFileSize)
		}

		// Sanity check on section size - if it's unreasonably large, something is wrong
		if tmpFileSize > 100*1024*1024 { // 100 MB limit as a reasonable maximum
			return fmt.Errorf("section %s size is unreasonably large (%d bytes), possible format error",
				section.Type, tmpFileSize)
		}

		// Prepare section header (64 bytes)
		sectionHeader := make([]byte, 64)

		// Set the section type at bytes 0-1 (2-byte big-endian)
		binary.BigEndian.PutUint16(sectionHeader[0:2], uint16(sectionType))

		// Bytes 2-7 are reserved (leave as zeros)

		// Set the section size at bytes 8-15 (8-byte big-endian)
		binary.BigEndian.PutUint64(sectionHeader[8:16], uint64(tmpFileSize))

		// Get current file position to calculate next section offset
		filePos, err := scState.OutFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("failed to get current file position for next section offset calculation: %w", err)
		}

		// Calculate next section offset (current position + header size + data size + padding)
		nextSectionPos := filePos + 64 + tmpFileSize // 64 bytes for header + section data
		padding := int64(0)
		if nextSectionPos%64 != 0 {
			padding = 64 - (nextSectionPos % 64)
		}
		nextOffset := nextSectionPos + padding

		// Set the next section offset at bytes 16-23 (8-byte big-endian)
		binary.BigEndian.PutUint64(sectionHeader[16:24], uint64(nextOffset))

		// Bytes 24-63 are additional metadata (leave as zeros for now)

		// Store the current position as the section's offset
		sectionOffset := currentPos
		scState.SectionFiles.UpdateOffset(section.Type, sectionOffset)

		// Write the section header (64 bytes)
		_, err = scState.OutFile.Write(sectionHeader)
		if err != nil {
			return fmt.Errorf("failed to write section header for %s: %w", section.Type, err)
		}

		// Get current file position after writing section header
		_, err = scState.OutFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("failed to get current file position after writing section header: %w", err)
		}
		fmt.Printf("Writing section %s, type: %d, size: %d bytes at offset %d\n",
			section.Type, sectionType, tmpFileSize, sectionOffset)

		// Debug: Print the section header
		fmt.Printf("Section %s header (64 bytes):\n", section.Type)
		printHexDump(sectionHeader, 0, 64, 0, 2, 8, 16, 24)

		// Reset the temporary file position to the beginning
		_, err = section.TmpFile.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek to beginning of temporary file for %s: %w", section.Type, err)
		}

		// Read the section data from the temporary file
		sectionData := make([]byte, tmpFileSize)
		_, err = io.ReadFull(section.TmpFile, sectionData)
		if err != nil {
			return fmt.Errorf("failed to read section data for %s: %w", section.Type, err)
		}

		// Debug: Print the first 64 bytes of the section data (or less if smaller)
		debugSize := int64(64)
		if tmpFileSize < debugSize {
			debugSize = tmpFileSize
		}
		fmt.Printf("First %d bytes of section %s data:\n", debugSize, section.Type)
		printHexDump(sectionData[:debugSize], 0, int(debugSize), 0, 0, 0, 0, 0)

		// Write the section data to the output file
		bytesWritten, err := scState.OutFile.Write(sectionData)
		if err != nil {
			return fmt.Errorf("failed to write section data for %s: %w", section.Type, err)
		}

		// Get current file position after writing section data
		currentPos, err = scState.OutFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("failed to get current file position after writing section data: %w", err)
		}

		// Check if the correct number of bytes was written
		if int64(bytesWritten) != tmpFileSize {
			return fmt.Errorf("incorrect number of bytes written for %s: expected %d, got %d",
				section.Type, tmpFileSize, bytesWritten)
		}

		fmt.Printf("Successfully wrote %d bytes of data for section %s\n", bytesWritten, section.Type)

		// Add padding to align to the next 64-byte boundary if needed
		if currentPos%64 != 0 {
			paddingSize := 64 - (currentPos % 64)
			padding := make([]byte, paddingSize)

			// Write padding
			_, err = scState.OutFile.Write(padding)
			if err != nil {
				return fmt.Errorf("failed to write alignment padding after section %s: %w", section.Type, err)
			}

			currentPos += int64(paddingSize)
			fmt.Printf("Added %d bytes of padding after section %s to align to 64-byte boundary\n",
				paddingSize, section.Type)
		}
	}

	fmt.Printf("All sections written successfully\n")

	// Print a summary of the reconstructed snapshot
	printSnapshotSummary(scState, scState.OutFile)

	return nil
}

// printSnapshotSummary prints a summary of the reconstructed snapshot
func printSnapshotSummary(scState *sc_State, file *os.File) error {
	// Get file size
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}
	totalSize := fileInfo.Size()

	fmt.Printf("\n=== Snapshot Reconstruction Summary ===\n")
	fmt.Printf("Total snapshot size: %d bytes (%.2f MB)\n", totalSize, float64(totalSize)/1024/1024)

	// Seek to the beginning of the file to read the header
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("error seeking to beginning of file: %w", err)
	}

	// Try to open the snapshot using the snapshot package
	snapshotReader, err := snapshot.Open(file)
	if err != nil {
		fmt.Printf("Error opening snapshot: %v\n", err)
		return err
	}

	// Print header information
	fmt.Printf("Snapshot version: %d\n", snapshotReader.Header.Version)
	fmt.Printf("Root hash: %x\n", snapshotReader.Header.RootHash)

	// If we have exactly one input snapshot, compare it with the output
	if len(scState.InputFiles) == 1 {
		inputFile := scState.InputFiles[0]

		// Get input file size
		inputInfo, err := inputFile.Stat()
		if err != nil {
			fmt.Printf("Error getting input file info: %v\n", err)
		} else {
			inputSize := inputInfo.Size()
			fmt.Printf("Input snapshot size: %d bytes (%.2f MB)\n", inputSize, float64(inputSize)/1024/1024)
			fmt.Printf("Output snapshot size: %d bytes (%.2f MB)\n", totalSize, float64(totalSize)/1024/1024)

			// Compare the files
			fmt.Printf("Comparing input and output snapshots...\n")

			// Reset file positions
			_, err = inputFile.Seek(0, io.SeekStart)
			if err != nil {
				fmt.Printf("Error seeking input file: %v\n", err)
				return nil
			}

			_, err = file.Seek(0, io.SeekStart)
			if err != nil {
				fmt.Printf("Error seeking output file: %v\n", err)
				return nil
			}

			// Compare the files byte by byte
			var firstDiffIndex int64 = -1
			var headerIndexes []int64
			var bufSize int64 = 4096
			inputBuf := make([]byte, bufSize)
			outputBuf := make([]byte, bufSize)

			// Collect header positions (every 64 bytes)
			for i := int64(0); i < minInt64(inputSize, totalSize); i += 64 {
				headerIndexes = append(headerIndexes, i)
			}

			// Compare in chunks
			for offset := int64(0); offset < minInt64(inputSize, totalSize); offset += bufSize {
				// Adjust buffer size for the last chunk
				currentBufSize := minInt64(bufSize, minInt64(inputSize, totalSize)-offset)

				// Read from both files
				_, err1 := io.ReadFull(inputFile, inputBuf[:currentBufSize])
				_, err2 := io.ReadFull(file, outputBuf[:currentBufSize])

				if err1 != nil || err2 != nil {
					fmt.Printf("Error reading files for comparison: %v, %v\n", err1, err2)
					break
				}

				// Compare buffers
				for i := int64(0); i < currentBufSize; i++ {
					if inputBuf[i] != outputBuf[i] {
						firstDiffIndex = offset + i
						fmt.Printf("Files differ at byte %d (0x%x)\n", firstDiffIndex, firstDiffIndex)
						break
					}
				}

				if firstDiffIndex >= 0 {
					break
				}
			}

			// If files differ, show the difference
			if firstDiffIndex >= 0 {
				fmt.Printf("Files differ! Showing context around first difference:\n")

				// Reset input file position
				_, err = inputFile.Seek(0, io.SeekStart)
				if err != nil {
					fmt.Printf("Error seeking input file: %v\n", err)
					return nil
				}

				// Calculate range to display (128 bytes before and after the difference)
				startOffset := maxInt64(int64(0), firstDiffIndex-128)
				endOffset := minInt64(inputSize, firstDiffIndex+128)
				displaySize := endOffset - startOffset

				// Read the range from input file
				_, err = inputFile.Seek(startOffset, io.SeekStart)
				if err != nil {
					fmt.Printf("Error seeking input file: %v\n", err)
					return nil
				}

				displayBuf := make([]byte, displaySize)
				_, err = io.ReadFull(inputFile, displayBuf)
				if err != nil {
					fmt.Printf("Error reading input file: %v\n", err)
					return nil
				}

				// Find relevant header indexes within the display range
				var relevantHeaders []int64
				for _, idx := range headerIndexes {
					if idx >= startOffset && idx < endOffset {
						relevantHeaders = append(relevantHeaders, idx-startOffset)
					}
				}

				// Print hex dump with the difference highlighted
				fmt.Printf("Context around first difference (offset 0x%x):\n", startOffset)

				// Convert int64 parameters to int for printHexDump
				intStartOffset := int(startOffset)
				intDiffIndex := int(firstDiffIndex - startOffset)

				// Convert relevantHeaders from []int64 to []int
				intRelevantHeaders := make([]int, len(relevantHeaders))
				for i, h := range relevantHeaders {
					intRelevantHeaders[i] = int(h)
				}

				printHexDump(displayBuf, intStartOffset, int(displaySize), append([]int{intDiffIndex}, intRelevantHeaders...)...)
			} else if inputSize != totalSize {
				fmt.Printf("Files have different sizes but matching content up to the smaller file's size.\n")
			} else {
				fmt.Printf("Files are identical!\n")
			}
		}
	}

	// Print section information
	fmt.Printf("Number of sections: %d\n", len(snapshotReader.Sections))
	for i, section := range snapshotReader.Sections {
		fmt.Printf("  Section %d: Type=%d, Size=%d bytes, Offset=0x%x\n",
			i, section.Type(), section.Size(), section.Offset())
	}

	fmt.Printf("=== End of Summary ===\n")
	return nil
}

// parseSectionType converts a section type string (e.g., "1_1") to a snapshot section type
func parseSectionType(sectionType string) (int, error) {
	parts := strings.Split(sectionType, "_")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid section type format: %s", sectionType)
	}

	// Parse the first part as the section type
	typeNum, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("failed to parse section type: %w", err)
	}

	// Map the section type to the snapshot package's section types
	switch typeNum {
	case 1:
		return int(snapshot.SectionTypeHeader), nil
	case 2:
		return int(snapshot.SectionTypeBPT), nil
	case 3:
		return int(snapshot.SectionTypeRecords), nil
	default:
		// For unknown types, return the parsed type as-is
		return typeNum, nil
	}
}

// minInt64 returns the smaller of two int64 values
func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// maxInt64 returns the larger of two int64 values
func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
