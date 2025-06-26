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
	"testing"
)

// TestAnalyzeSectionLayout analyzes the layout of sections in the original snapshot
// to understand the exact structure and offsets of each section.
func TestAnalyzeSectionLayout(t *testing.T) {
	// Define the path to the test snapshot file
	snapshotPath := "/home/paul/work/acc1/dn.snap"
	_, err := os.Stat(snapshotPath)
	if os.IsNotExist(err) {
		t.Skipf("Skipping test: snapshot file not found at %s", snapshotPath)
		return
	} else if err != nil {
		t.Fatalf("Failed to check if snapshot file exists: %v", err)
	}

	// Open the snapshot file
	file, err := os.Open(snapshotPath)
	if err != nil {
		t.Fatalf("Failed to open snapshot file: %v", err)
	}
	defer file.Close()

	// Read and analyze each section
	fmt.Println("=== Original Snapshot Section Analysis ===")
	
	var currentOffset int64 = 0
	sectionCount := 0
	
	for {
		// Read the section header (64 bytes)
		header := make([]byte, 64)
		_, err = file.Seek(currentOffset, io.SeekStart)
		if err != nil {
			t.Fatalf("Failed to seek to section header at offset 0x%x: %v", currentOffset, err)
		}
		
		n, err := file.Read(header)
		if err != nil || n < 64 {
			if err == io.EOF {
				break
			}
			t.Fatalf("Failed to read section header at offset 0x%x: %v", currentOffset, err)
		}
		
		// Extract section type (first 2 bytes, big-endian)
		sectionType := binary.BigEndian.Uint16(header[0:2])
		
		// Extract section size (bytes 8-15, big-endian)
		sectionSize := binary.BigEndian.Uint64(header[8:16])
		
		// Extract next section offset (bytes 16-23, big-endian)
		nextSectionOffset := binary.BigEndian.Uint64(header[16:24])
		
		fmt.Printf("Section #%d at offset 0x%x:\n", sectionCount, currentOffset)
		fmt.Printf("  Type: %d\n", sectionType)
		fmt.Printf("  Size: %d (0x%x) bytes\n", sectionSize, sectionSize)
		fmt.Printf("  Next Section Offset: 0x%x\n", nextSectionOffset)
		
		// Calculate the actual data size (next section offset - current offset - header size)
		var actualDataSize int64
		if nextSectionOffset > 0 {
			actualDataSize = int64(nextSectionOffset) - currentOffset - 64
			fmt.Printf("  Actual Data Size: %d bytes\n", actualDataSize)
			
			// Check if there's padding
			if actualDataSize > int64(sectionSize) {
				paddingSize := actualDataSize - int64(sectionSize)
				fmt.Printf("  Padding Size: %d bytes\n", paddingSize)
			}
		} else {
			// Last section
			fmt.Printf("  Last section (no next offset)\n")
		}
		
		// Print the first 16 bytes of the section data
		if sectionType > 0 {
			dataPreview := make([]byte, 16)
			_, err = file.Seek(currentOffset+64, io.SeekStart)
			if err != nil {
				t.Fatalf("Failed to seek to section data at offset 0x%x: %v", currentOffset+64, err)
			}
			
			n, err := file.Read(dataPreview)
			if err != nil && err != io.EOF {
				t.Fatalf("Failed to read section data at offset 0x%x: %v", currentOffset+64, err)
			}
			
			fmt.Printf("  Data Preview: ")
			for i := 0; i < n; i++ {
				fmt.Printf("%02x ", dataPreview[i])
			}
			fmt.Println()
		}
		
		fmt.Println()
		
		// Move to the next section
		sectionCount++
		if nextSectionOffset == 0 {
			break
		}
		currentOffset = int64(nextSectionOffset)
	}
	
	fmt.Printf("Total sections found: %d\n", sectionCount)
}
