// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Note: Section type constants are defined in snap_combiner.go

// sc_ParseSnapshot reads all sections from the snapshot file and writes records to temporary files
func sc_ParseSnapshot(state *sc_State) error {
	// Reset the file position to the beginning
	_, err := state.File.Seek(0, io.SeekStart)
	if err != nil {
		return sc_recordError(state, "seek_error", fmt.Errorf("failed to seek to beginning of file: %w", err))
	}

	// Check for snapshot format version and get the first section offset
	if err := sc_checkSnapshotVersion(state); err != nil {
		return err
	}
	
	// Initialize the original sections slice
	state.OriginalSections = make([]SectionInfo, 0)

	fmt.Println("Parsing snapshot file...")

	// Reset the file position to the beginning again to start parsing sections
	_, err = state.File.Seek(0, io.SeekStart)
	if err != nil {
		return sc_recordError(state, "seek_error", fmt.Errorf("failed to seek to beginning of file: %w", err))
	}

	// Read sections until EOF or until nextOffset is 0
	var currentOffset int64 = 0
	var sectionOrder int = 0
	
	// Track section instances (for sections that appear multiple times)
	sectionInstances := make(map[uint32]int)
	
	for {
		// Seek to the current section header
		_, err := state.File.Seek(currentOffset, io.SeekStart)
		if err != nil {
			return sc_recordError(state, "seek_error", fmt.Errorf("failed to seek to section at offset %d: %w", currentOffset, err))
		}

		// Read section header (64 bytes)
		header, err := sc_readSectionHeader(state)
		if err != nil {
			if err == io.EOF {
				// End of file reached
				break
			}
			return sc_recordError(state, "header_read_error", fmt.Errorf("failed to read section header at offset %d: %w", currentOffset, err))
		}

		// Convert section type to uint32 for compatibility with existing code
		sectionType := uint32(header.Type)
		sectionSize := header.Size

		// Update statistics
		state.TotalSections++
		state.SectionSizes[sectionType] += int64(sectionSize)
		
		// Log section info
		fmt.Printf("Processing section type: %d (%s), size: %d bytes, at offset: %d\n", 
			header.Type, sc_getSectionTypeName(header.Type), sectionSize, header.ContentOffset)
		
		// Track original section information
		// Increment the instance counter for this section type
		sectionInstances[uint32(header.Type)]++
		
		sectionInfo := SectionInfo{
			Type:        uint32(header.Type),
			StartOffset: header.HeaderOffset,
			HeaderOffset: header.HeaderOffset,
			DataOffset:  header.ContentOffset,
			Size:        sectionSize,
			EndOffset:   header.HeaderOffset + int64(sectionSize) + 64, // header + data
			Order:       sectionOrder,
			Instance:    sectionInstances[uint32(header.Type)],
		}
		state.OriginalSections = append(state.OriginalSections, sectionInfo)
		sectionOrder++

		// Create or get the temporary file for this section type
		tmpFile, err := sc_getOrCreateSectionFile(state, sectionType)
		if err != nil {
			return sc_recordError(state, "temp_file_error", err)
		}

		// Process the section data
		recordCount, err := sc_processSectionData(state, tmpFile, sectionType, sectionSize)
		if err != nil {
			return err
		}
		
		// Update record count for this section type
		state.SectionCounts[sectionType] += recordCount
		state.TotalRecords += recordCount

		// Log completion of section processing
		fmt.Printf("Section type %d processed: %d records\n", sectionType, recordCount)

		// Move to the next section
		if header.NextOffset == 0 {
			// No more sections
			break
		}
		currentOffset = int64(header.NextOffset)
	}

	fmt.Println("Snapshot parsing completed")
	return nil
}

// sc_processSectionData reads and processes section data based on section type
// Returns the number of records processed in the section
func sc_processSectionData(state *sc_State, tmpFile *os.File, sectionType uint32, sectionSize uint64) (int, error) {
	// Save the current position to restore it later
	currentPos, err := state.File.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, sc_recordError(state, "seek_error", fmt.Errorf("failed to get current position: %w", err))
	}
	
	// Process the section based on its type
	var recordCount int
	
	switch sectionType {
	case 1: // SectionTypeHeader
		// Header section is already processed in sc_checkSnapshotVersion
		// Just skip the content
		_, err = state.File.Seek(int64(sectionSize), io.SeekCurrent)
		if err != nil {
			return 0, sc_recordError(state, "seek_error", fmt.Errorf("failed to skip header content: %w", err))
		}
		recordCount = 1 // Count the header as one record
		
	case 7: // SectionTypeRecords
		// Create a section header for the records section
		header := &sc_SectionHeader{
			Type:          uint16(sectionType),
			Size:          sectionSize,
			ContentOffset: currentPos,
		}
		
		// Process the records section
		var err error
		recordCount, err = processRecordsSection(state.File, header, tmpFile)
		if err != nil {
			return 0, sc_recordError(state, "records_processing_error", err)
		}
		
	case 8: // SectionTypeRecordIndex
		// Create a section header for the record index section
		header := &sc_SectionHeader{
			Type:          uint16(sectionType),
			Size:          sectionSize,
			ContentOffset: currentPos,
		}
		
		// Process the record index section
		var err error
		recordCount, err = processRecordIndexSection(state.File, header, tmpFile)
		if err != nil {
			return 0, sc_recordError(state, "record_index_processing_error", err)
		}
		
	case 11: // SectionTypeBPT
		// Create a section header for the BPT section
		header := &sc_SectionHeader{
			Type:          uint16(sectionType),
			Size:          sectionSize,
			ContentOffset: currentPos,
		}
		
		// Process the BPT section
		var err error
		recordCount, err = processBPTSection(state.File, header, tmpFile)
		if err != nil {
			return 0, sc_recordError(state, "bpt_processing_error", err)
		}
		
	default:
		// For other section types, just copy the data to the temporary file
		// Read and write the section data in chunks to avoid loading everything into memory
		const chunkSize = 1024 * 1024 // 1MB chunks
		remaining := sectionSize
		buffer := make([]byte, chunkSize)
		
		// Read and write the section data in chunks
		for remaining > 0 {
			// Read a chunk (or the remaining bytes if less than chunk size)
			readSize := remaining
			if readSize > chunkSize {
				readSize = chunkSize
			}
			
			// Read the chunk
			n, err := io.ReadFull(state.File, buffer[:readSize])
			if err != nil {
				if err == io.EOF {
					// End of file reached (shouldn't happen in the middle of a section)
					return 0, sc_recordError(state, "unexpected_eof", 
						fmt.Errorf("unexpected EOF while reading section data: %d bytes remaining", remaining))
				}
				return 0, sc_recordError(state, "read_error", fmt.Errorf("failed to read section data: %w", err))
			}
			
			// Write the chunk to the temporary file
			_, err = tmpFile.Write(buffer[:n])
			if err != nil {
				return 0, sc_recordError(state, "write_error", fmt.Errorf("failed to write section data: %w", err))
			}
			
			// Update remaining bytes
			remaining -= uint64(n)
		}
		
		// Estimate record count based on section size
		recordCount = 1 // Default to 1 record for unknown section types
	}
	
	return recordCount, nil
}

// sc_recordError records an error in the state's error counts and returns the error
func sc_recordError(state *sc_State, errorType string, err error) error {
	state.ErrorCounts[errorType]++
	return err
}

// sc_SectionHeader represents a snapshot section header
type sc_SectionHeader struct {
	Type            uint16 // Section type (2 bytes)
	Size            uint64 // Section size (8 bytes)
	NextOffset      uint64 // Offset to next section (8 bytes)
	HeaderOffset    int64  // Offset of this header in the file
	ContentOffset   int64  // Offset of the section content (header + 64)
}

// sc_readSectionHeader reads a section header from the current position in the file
func sc_readSectionHeader(state *sc_State) (*sc_SectionHeader, error) {
	// Get current position
	currentPos, err := state.File.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, fmt.Errorf("failed to get current position: %w", err)
	}
	
	// Read the 64-byte header
	headerBytes := make([]byte, 64)
	n, err := io.ReadFull(state.File, headerBytes)
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			// End of file reached
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to read section header: %w", err)
	}
	if n != 64 {
		return nil, fmt.Errorf("incomplete section header: read %d bytes, expected 64", n)
	}
	
	// Parse the header fields
	header := &sc_SectionHeader{
		Type:           binary.BigEndian.Uint16(headerBytes[0:2]),
		Size:           binary.BigEndian.Uint64(headerBytes[8:16]),
		NextOffset:     binary.BigEndian.Uint64(headerBytes[16:24]),
		HeaderOffset:   currentPos,
		ContentOffset:  currentPos + 64,
	}
	
	// Validate the section size
	if header.Size > 1024*1024*1024 { // 1GB sanity check
		return nil, fmt.Errorf("section size too large: %d bytes", header.Size)
	}
	
	return header, nil
}

// sc_checkSnapshotVersion checks if the snapshot file has a valid format version
func sc_checkSnapshotVersion(state *sc_State) error {
	// Read the first 64 bytes to check for a valid header
	headerBytes := make([]byte, 64)
	_, err := io.ReadFull(state.File, headerBytes)
	if err != nil {
		return sc_recordError(state, "header_read_error", fmt.Errorf("failed to read snapshot header: %w", err))
	}
	
	// Reset position after reading
	_, err = state.File.Seek(0, io.SeekStart)
	if err != nil {
		return sc_recordError(state, "seek_error", fmt.Errorf("failed to reset file position: %w", err))
	}
	
	// Print the first 64 bytes for diagnostic purposes
	fmt.Println("Snapshot header bytes:")
	for i := 0; i < 64; i += 16 {
		end := i + 16
		if end > 64 {
			end = 64
		}
		fmt.Printf("%04x: % x\n", i, headerBytes[i:end])
	}
	
	// The snapshot format uses 64-byte section headers with the following format:
	// - Bytes 0-1: Section type (uint16 in big-endian format)
	// - Bytes 2-7: Reserved (6 bytes)
	// - Bytes 8-15: Section size (uint64 in big-endian format)
	// - Bytes 16-23: Next section offset (uint64 in big-endian format)
	// - Bytes 24-63: Additional metadata (40 bytes)
	
	// Parse the section type (first 2 bytes, big-endian)
	sectionType := binary.BigEndian.Uint16(headerBytes[0:2])
	fmt.Printf("Section type: %d\n", sectionType)
	
	// The first section must be a header section (type 1)
	if sectionType != 1 { // SectionTypeHeader
		return sc_recordError(state, "invalid_header", 
			fmt.Errorf("invalid first section type: %d (expected 1 for header)", sectionType))
	}
	
	// Parse the section size (bytes 8-15, big-endian)
	sectionSize := binary.BigEndian.Uint64(headerBytes[8:16])
	fmt.Printf("Section size: %d bytes\n", sectionSize)
	
	// Parse the next section offset (bytes 16-23, big-endian)
	nextSectionOffset := binary.BigEndian.Uint64(headerBytes[16:24])
	fmt.Printf("Next section offset: %d bytes\n", nextSectionOffset)
	
	// Store the first section offset in the state
	state.FirstSectionOffset = nextSectionOffset
	
	// Read the header content (which follows the 64-byte section header)
	// The header content contains the format version
	headerContent := make([]byte, sectionSize)
	_, err = io.ReadFull(state.File, headerContent)
	if err != nil {
		return sc_recordError(state, "header_content_error", 
			fmt.Errorf("failed to read header content: %w", err))
	}
	
	// Reset position after reading
	_, err = state.File.Seek(0, io.SeekStart)
	if err != nil {
		return sc_recordError(state, "seek_error", fmt.Errorf("failed to reset file position: %w", err))
	}
	
	// The header content should contain the format version as a uint32 in big-endian format
	if len(headerContent) < 4 {
		return sc_recordError(state, "invalid_header_content", 
			fmt.Errorf("header content too small: %d bytes", len(headerContent)))
	}
	
	// Parse the format version (first 4 bytes of header content, big-endian)
	formatVersion := binary.BigEndian.Uint32(headerContent[0:4])
	fmt.Printf("Detected snapshot format version: %d\n", formatVersion)
	
	// Store the format version in the state for future reference
	state.FormatVersion = formatVersion
	
	return nil
}

// sc_getSectionTypeName returns a human-readable name for a section type
func sc_getSectionTypeName(sectionType uint16) string {
	// Map section types to names based on the snapshot.SectionType constants
	switch sectionType {
	case 1: // SectionTypeHeader
		return "header"
	case 2: // SectionTypeAccountsV1
		return "accountsV1"
	case 3: // SectionTypeTransactionsV1
		return "transactionsV1"
	case 4: // SectionTypeSignaturesV1
		return "signaturesV1"
	case 5: // SectionTypeGzTransactionsV1
		return "gzTransactionsV1"
	case 6: // SectionTypeSnapshot
		return "snapshot"
	case 7: // SectionTypeRecords
		return "records"
	case 8: // SectionTypeRecordIndex
		return "recordIndex"
	case 9: // SectionTypeRawBPT
		return "rawBPT"
	case 10: // SectionTypeConsensus
		return "consensus"
	case 11: // SectionTypeBPT
		return "bpt"
	default:
		return fmt.Sprintf("unknown(%d)", sectionType)
	}
}

// sc_getOrCreateSectionFile returns an existing file handle for the section type or creates a new one
func sc_getOrCreateSectionFile(state *sc_State, sectionType uint32) (*os.File, error) {
	// Check if we already have a file for this section type
	if file, exists := state.SectionFiles[sectionType]; exists {
		return file, nil
	}

	// Create a new file for this section type
	fileName := fmt.Sprintf("section_%d.tmp", sectionType)
	filePath := filepath.Join(state.TempDir, fileName)
	
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file for section %d: %w", sectionType, err)
	}

	// Store the file handle in the map
	state.SectionFiles[sectionType] = file
	
	return file, nil
}
