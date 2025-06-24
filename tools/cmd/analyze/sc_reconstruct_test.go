package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestScReconstruct tests the snapshot reconstruction functionality
// using a real snapshot file
func TestScReconstruct(t *testing.T) {
	// Path to the test snapshot file
	sourceSnapshot := "/home/paul/work/acc1/dn.snap"

	// Get source file size
	sourceInfo, err := os.Stat(sourceSnapshot)
	if err != nil {
		t.Fatalf("Failed to get source file info: %v", err)
	}
	fmt.Printf("Source snapshot: %s (%d bytes)\n", sourceSnapshot, sourceInfo.Size())

	// Create a temporary directory for the test output
	tempDir, err := os.MkdirTemp("", "sc-reconstruct-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir) // Clean up after the test

	// Path to destination snapshot
	destSnapshot := filepath.Join(tempDir, "reconstructed.snap")

	// Create a mock cobra command to simulate CLI usage
	cmd := &cobra.Command{}

	// Set up arguments as they would be passed from the CLI
	// First argument is the output path, second is the input snapshot
	args := []string{destSnapshot, sourceSnapshot}

	err = sc_Run(cmd, args)
	if err != nil {
		t.Fatalf("Reconstruction failed: %v", err)
	}

	// Verify the output file exists and has content
	if _, err := os.Stat(destSnapshot); os.IsNotExist(err) {
		t.Errorf("Output file was not created at %s", destSnapshot)
	} else {
		// Get file size
		fileInfo, err := os.Stat(destSnapshot)
		if err != nil {
			t.Errorf("Failed to get output file info: %v", err)
		} else {
			fmt.Printf("Reconstructed snapshot: %s (%d bytes)\n", destSnapshot, fileInfo.Size())

			// Print size comparison
			sizeDiff := fileInfo.Size() - sourceInfo.Size()
			if sizeDiff == 0 {
				fmt.Println("File sizes match exactly!")
			} else {
				fmt.Printf("Size difference: %+d bytes (%.2f%%)\n",
					sizeDiff, float64(sizeDiff)*100/float64(sourceInfo.Size()))
			}

			// Basic validation: file should not be empty
			if fileInfo.Size() == 0 {
				t.Errorf("Reconstructed file is empty")
			}
		}
	}

	fmt.Println("Test completed successfully")
}

// TestPrintHexDump tests the printHexDump function with highlighted indexes
func TestPrintHexDump(t *testing.T) {
	// Create a RandHash instance
	var rh common.RandHash

	// Generate a 5KB buffer using RandHash
	bufferSize := 5 * 1024 // 5KB
	buffer := rh.GetRandBuff(bufferSize)

	// Run 10 test cases
	for i := 0; i < 10; i++ {
		fmt.Printf("\n\nTest Case %d:\n", i+1)
		fmt.Printf("-----------\n")

		// Generate test case
		startOffset, redIndex, indexes := generateTestCase(&rh, bufferSize)

		// Print information about the test case
		fmt.Printf("Buffer size: %d bytes\n", bufferSize)
		fmt.Printf("Display offset: %d\n", startOffset)
		fmt.Printf("Display range: %d to %d\n", startOffset, startOffset+255)
		fmt.Printf("Red index (first in list): %d\n", redIndex)

		// Find which indexes are within the visible range
		visibleIndexes := make([]int, 0)
		for _, idx := range indexes[1:] { // Skip the red index
			if idx >= startOffset && idx < startOffset+256 {
				visibleIndexes = append(visibleIndexes, idx)
			}
		}
		fmt.Printf("Visible green indexes: %v\n\n", visibleIndexes)

		// Call printHexDump with the indexes
		printHexDump(buffer, startOffset, 256, indexes...)
	}
}

func TestCompareHeaderEncoding(t *testing.T) {
	// First, dump the first 256 bytes of the original snapshot file
	sourceSnapshot := "/home/paul/work/acc1/dn.snap"

	// Open the original snapshot file
	originalFile, err := os.Open(sourceSnapshot)
	if err != nil {
		t.Fatalf("Failed to open original snapshot file: %v", err)
	}
	defer originalFile.Close()

	// Read the first 256 bytes
	originalBytes := make([]byte, 256)
	n, err := io.ReadFull(originalFile, originalBytes)
	if err != nil && err != io.ErrUnexpectedEOF {
		t.Fatalf("Failed to read original snapshot file: %v", err)
	}

	// If the file is smaller than 256 bytes, adjust the slice
	if n < 256 {
		originalBytes = originalBytes[:n]
	}

	fmt.Println("=== First 256 bytes of original snapshot file (dn.snap) ===")
	printHexDump(originalBytes, 0, -1)

	// Extract and display the header section from the original snapshot file
	// First 64 bytes are the file header
	fileHeader := originalBytes[:64]

	// Parse the section size from the header
	sectionSize := binary.BigEndian.Uint64(fileHeader[8:16])

	// Read the header section data
	_, err = originalFile.Seek(64, io.SeekStart) // Go back to position after file header
	if err != nil {
		t.Fatalf("Failed to seek in original snapshot file: %v", err)
	}

	// Read the header section data
	originalHeaderData := make([]byte, sectionSize)
	_, err = io.ReadFull(originalFile, originalHeaderData)
	if err != nil {
		t.Fatalf("Failed to read header section data: %v", err)
	}

	fmt.Println("\n=== Original File Header (64 bytes) ===")
	printHexDump(fileHeader, 0, -1)

	fmt.Println("\n=== Original Header Section Data ===")
	printHexDump(originalHeaderData, 0, -1)

	// Calculate padding in the original file
	originalTotalSize := 64 + int(sectionSize) // file header + header section data
	originalPadding := (64 - (originalTotalSize % 64)) % 64

	fmt.Printf("\nOriginal header section size: %d bytes\n", sectionSize)
	fmt.Printf("Original total size (file header + header section): %d bytes\n", originalTotalSize)
	fmt.Printf("Original padding needed: %d bytes\n", originalPadding)
	// Create a test header with sample data
	rootHash := [32]byte{}
	for i := 0; i < 32; i++ {
		rootHash[i] = byte(i)
	}

	// Parse the URL string into a URL object
	systemUrl, err := url.Parse("acc://system")
	if err != nil {
		t.Fatalf("Failed to parse URL: %v", err)
	}

	systemLedger := &protocol.SystemLedger{
		Url:   systemUrl,
		Index: 1,
	}

	// ==========================================
	// Our approach: Using sc_WriteSectionType1
	// ==========================================
	var ourBuffer bytes.Buffer

	// Create an encoding writer that follows the same format as snapshot.Header.MarshalBinary
	writer := encoding.NewWriter(&ourBuffer)

	// Write field 1: Version = 2
	writer.WriteUint(1, 2) // Field 1, Value 2 (format version)

	// Write field 2: Root hash
	writer.WriteHash(2, &rootHash)

	// Write field 3: System ledger
	systemLedgerData, err := systemLedger.MarshalBinary()
	if err != nil {
		t.Fatalf("Failed to marshal system ledger: %v", err)
	}
	writer.WriteBytes(3, systemLedgerData)

	// Finalize the writer
	_, _, err = writer.Reset(nil)
	if err != nil {
		t.Fatalf("Failed to finalize header encoding: %v", err)
	}

	// Get the encoded header data
	ourHeaderData := ourBuffer.Bytes()

	// ==========================================
	// Debug snap collect approach: Using snapshot.Header
	// ==========================================
	// Create a snapshot header
	header := &snapshot.Header{
		Version:      2,
		RootHash:     rootHash,
		SystemLedger: systemLedger,
	}

	// Marshal the header using the snapshot package
	debugHeaderData, err := header.MarshalBinary()
	if err != nil {
		t.Fatalf("Failed to marshal header: %v", err)
	}

	// ==========================================
	// Our approach: Calculate section header and padding
	// ==========================================
	// Create our section header (64 bytes)
	var ourSectionHeader [64]byte

	// Set section type to 1 (header section)
	binary.BigEndian.PutUint64(ourSectionHeader[0:8], 1)

	// Set section size
	binary.BigEndian.PutUint64(ourSectionHeader[8:16], uint64(len(ourHeaderData)))

	// Calculate padding needed to align next section to 64-byte boundary
	ourTotalSize := 64 + len(ourHeaderData) // section header + header data
	ourPadding := (64 - (ourTotalSize % 64)) % 64

	// Calculate offset to next section
	ourNextOffset := uint64(ourTotalSize + ourPadding)
	binary.BigEndian.PutUint64(ourSectionHeader[16:24], ourNextOffset)

	// Create our padding bytes
	ourPaddingBytes := make([]byte, ourPadding)

	// ==========================================
	// Debug snap collect approach: Calculate section header and padding
	// ==========================================
	// Create debug section header (64 bytes)
	var debugSectionHeader [64]byte

	// Set section type to 1 (header section)
	binary.BigEndian.PutUint64(debugSectionHeader[0:8], 1)

	// Set section size
	binary.BigEndian.PutUint64(debugSectionHeader[8:16], uint64(len(debugHeaderData)))

	// Calculate padding needed to align next section to 64-byte boundary
	debugTotalSize := 64 + len(debugHeaderData) // section header + header data
	debugPadding := (64 - (debugTotalSize % 64)) % 64

	// Calculate offset to next section
	debugNextOffset := uint64(debugTotalSize + debugPadding)
	binary.BigEndian.PutUint64(debugSectionHeader[16:24], debugNextOffset)

	// Create debug padding bytes
	debugPaddingBytes := make([]byte, debugPadding)

	// ==========================================
	// Print hex dumps for comparison
	// ==========================================
	fmt.Println("\n=== Our Approach ===")
	fmt.Println("=== Our Header Section Data ===")
	printHexDump(ourHeaderData, 0, -1)

	fmt.Println("\n=== Our Section Header (64 bytes) ===")
	printHexDump(ourSectionHeader[:], 0, -1)

	fmt.Println("\n=== Our Padding Bytes ===")
	printHexDump(ourPaddingBytes, 0, -1)

	fmt.Printf("\nOur header data size: %d bytes\n", len(ourHeaderData))
	fmt.Printf("Our total size (section header + header data): %d bytes\n", ourTotalSize)
	fmt.Printf("Our padding needed: %d bytes\n", ourPadding)
	fmt.Printf("Our next section offset: %d bytes\n", ourNextOffset)

	fmt.Println("\n=== Debug Snap Collect Approach ===")
	fmt.Println("=== Debug Header Section Data ===")
	printHexDump(debugHeaderData, 0, -1)

	fmt.Println("\n=== Debug Section Header (64 bytes) ===")
	printHexDump(debugSectionHeader[:], 0, -1)

	fmt.Println("\n=== Debug Padding Bytes ===")
	printHexDump(debugPaddingBytes, 0, -1)

	fmt.Printf("\nDebug header data size: %d bytes\n", len(debugHeaderData))
	fmt.Printf("Debug total size (section header + header data): %d bytes\n", debugTotalSize)
	fmt.Printf("Debug padding needed: %d bytes\n", debugPadding)
	fmt.Printf("Debug next section offset: %d bytes\n", debugNextOffset)

	// Compare the header data encodings
	fmt.Println("\n=== Comparison Results ===")
	fmt.Println("1. Header Section Data:")
	if bytes.Equal(ourHeaderData, debugHeaderData) {
		fmt.Println("   The header data encodings are identical!")
	} else {
		fmt.Println("   The header data encodings are different!")

		// Find the first difference
		minLen := len(ourHeaderData)
		if len(debugHeaderData) < minLen {
			minLen = len(debugHeaderData)
		}

		for i := 0; i < minLen; i++ {
			if ourHeaderData[i] != debugHeaderData[i] {
				fmt.Printf("     First difference at byte %d: our: %02X, debug: %02X\n",
					i, ourHeaderData[i], debugHeaderData[i])
				break
			}
		}

		if len(ourHeaderData) != len(debugHeaderData) {
			fmt.Printf("     Length difference: our: %d bytes, debug: %d bytes\n",
				len(ourHeaderData), len(debugHeaderData))
		}
	}

	// Compare the section headers
	fmt.Println("\n2. Section Headers:")
	if bytes.Equal(ourSectionHeader[:], debugSectionHeader[:]) {
		fmt.Println("   The section headers are identical!")
	} else {
		fmt.Println("   The section headers are different!")

		// Find the first difference in section headers
		for i := 0; i < 64; i++ {
			if ourSectionHeader[i] != debugSectionHeader[i] {
				fmt.Printf("     First difference at byte %d: our: %02X, debug: %02X\n",
					i, ourSectionHeader[i], debugSectionHeader[i])
				break
			}
		}
	}

	// Compare the padding
	fmt.Println("\n3. Padding:")
	if ourPadding == debugPadding {
		fmt.Printf("   The padding sizes are identical! (%d bytes)\n", ourPadding)
	} else {
		fmt.Printf("   The padding sizes are different! (our: %d bytes, debug: %d bytes)\n",
			ourPadding, debugPadding)
	}

	// Compare the next section offsets
	fmt.Println("\n4. Next Section Offsets:")
	if ourNextOffset == debugNextOffset {
		fmt.Printf("   \u2713 The next section offsets are identical! (%d bytes)\n", ourNextOffset)
	} else {
		fmt.Printf("   \u2717 The next section offsets are different! (our: %d bytes, debug: %d bytes)\n",
			ourNextOffset, debugNextOffset)
	}

	// Compare our encoding with the original header section data
	fmt.Println("\n5. Comparison with Original Snapshot:")
	if bytes.Equal(ourHeaderData, originalHeaderData) {
		fmt.Println("   \u2713 Our header data matches the original snapshot file!")
	} else {
		fmt.Println("   \u2717 Our header data differs from the original snapshot file!")

		// Find the first difference
		minLen := len(ourHeaderData)
		if len(originalHeaderData) < minLen {
			minLen = len(originalHeaderData)
		}

		for i := 0; i < minLen; i++ {
			if ourHeaderData[i] != originalHeaderData[i] {
				fmt.Printf("     First difference at byte %d: our: %02X, original: %02X\n",
					i, ourHeaderData[i], originalHeaderData[i])
				break
			}
		}

		if len(ourHeaderData) != len(originalHeaderData) {
			fmt.Printf("     Length difference: our: %d bytes, original: %d bytes\n",
				len(ourHeaderData), len(originalHeaderData))
		}
	}

	// Compare debug encoding with the original header section data
	if bytes.Equal(debugHeaderData, originalHeaderData) {
		fmt.Println("   \u2713 Debug header data matches the original snapshot file!")
	} else {
		fmt.Println("   \u2717 Debug header data differs from the original snapshot file!")

		// Find the first difference
		minLen := len(debugHeaderData)
		if len(originalHeaderData) < minLen {
			minLen = len(originalHeaderData)
		}

		for i := 0; i < minLen; i++ {
			if debugHeaderData[i] != originalHeaderData[i] {
				fmt.Printf("     First difference at byte %d: debug: %02X, original: %02X\n",
					i, debugHeaderData[i], originalHeaderData[i])
				break
			}
		}

		if len(debugHeaderData) != len(originalHeaderData) {
			fmt.Printf("     Length difference: debug: %d bytes, original: %d bytes\n",
				len(debugHeaderData), len(originalHeaderData))
		}
	}
}

// generateTestCase creates a test case for printHexDump according to specifications
func generateTestCase(rh *common.RandHash, bufferSize int) (startOffset, redIndex int, indexes []int) {
	// Pick a random offset within the 5KB buffer, leaving room for 256 bytes
	startOffset = rh.GetIntN(bufferSize - 256)

	// Pick a red index somewhere in the 256 bytes after the offset
	redIndex = startOffset + rh.GetIntN(256)

	// Generate 12 offsets that are outside the 256 bytes after the offset
	outsideIndexes := make([]int, 0, 12)
	for len(outsideIndexes) < 12 {
		// Generate an index either before the start offset or after the end of the window
		var idx int
		if rh.GetIntN(2) == 0 && startOffset > 0 { // 50% chance, if possible
			// Generate an index before the start offset
			idx = rh.GetIntN(startOffset)
		} else {
			// Generate an index after the end of the window
			idx = startOffset + 256 + rh.GetIntN(bufferSize-(startOffset+256))
		}

		// Check if this index is unique
		isUnique := true
		for _, existingIdx := range outsideIndexes {
			if idx == existingIdx {
				isUnique = false
				break
			}
		}

		if isUnique {
			outsideIndexes = append(outsideIndexes, idx)
		}
	}

	// Generate one index within the visible range (sometimes aligned to 64 bytes)
	var visibleIndex int
	if rh.GetIntN(2) == 0 { // 50% chance of alignment to 64 bytes
		// Calculate how many 64-byte blocks fit in our window
		numBlocks := 256 / 64
		// Pick a random block
		block := rh.GetIntN(numBlocks)
		// Align to the start of that block
		visibleIndex = startOffset + (block * 64)
	} else {
		// Just pick a random index in the visible range
		visibleIndex = startOffset + rh.GetIntN(256)
	}

	// Replace one of the outside indexes with the visible index
	replaceIdx := rh.GetIntN(len(outsideIndexes))
	outsideIndexes[replaceIdx] = visibleIndex

	// Combine the red index and the outside indexes (with one replaced)
	indexes = append([]int{redIndex}, outsideIndexes...)

	return startOffset, redIndex, indexes
}
