# Snapshot Parser Design

This document outlines the design of the memory-efficient snapshot parser implemented in the `sc` command. The parser is designed to process large snapshot files by streaming section data to temporary files, avoiding excessive memory usage.

## Overview

The snapshot parser reads snapshot files section by section, writing each section's data to a separate temporary file. This approach allows processing of very large snapshot files without loading the entire file into memory.

## Snapshot File Format

### Segmented Writer Algorithm

The snapshot file is created using a segmented writer that follows this precise algorithm for each section:

1. **Opening a Section**:
   - If a previous section exists, update its "next section offset" field (bytes 16-23) with the current file position
   - Write a 64-byte placeholder for the section header
   - Record the current file position as the section data start

2. **Writing Section Data**:
   - Write the section data to the file
   - Track the total bytes written

3. **Closing a Section**:
   - Calculate the current file position after writing data
   - Seek back to the section header position
   - Write the section header with:
     - Section type (bytes 0-1): Set to the section's type value
     - Section size (bytes 8-15): Set to the exact data size (excluding header and padding)
     - Next section offset (bytes 16-23): Set to 0 (will be updated when the next section is opened)
   - Seek back to the end of the section data
   - Add padding bytes if needed to align to the next 64-byte boundary using the formula:
     `padding_size = (64 - (current_position % 64)) % 64`
   - Update the file position to after the padding
   - Record this position as the start of the next section

This algorithm ensures that:
- All sections are properly aligned on 64-byte boundaries
- The "next section offset" field in each section header points to the exact file position where the next section begins
- The section size reflects only the actual data size, not including the header or padding

### File Structure

The Accumulate snapshot file uses a segmented file format with the following characteristics:

1. **Main Snapshot Header**: A 64-byte header at the beginning of the file
2. **Section Headers**: Each section has a 64-byte header
3. **Section Data**: Variable-length data following each section header
4. **Alignment**: Sections are aligned on 64-byte boundaries

### Main Snapshot Header (First 64 bytes)

The main snapshot header contains:

- Bytes 0-1: Section type (0x0001 for the main header)
- Bytes 2-7: Reserved (zeros)
- Bytes 8-15: Size of the header data (0x0000000000000079 in the example)
- Bytes 16-23: First section offset (0x00000000000000C0 = 192 in the example)
- Bytes 24-63: Additional metadata (zeros)

### Section Headers (64 bytes each)

Each section header follows the same format:

- Bytes 0-1: Section type (uint16 in big-endian format)
- Bytes 2-7: Reserved (6 bytes)
- Bytes 8-15: Section size (uint64 in big-endian format) - Exact data size excluding header and padding
- Bytes 16-23: Next section offset (uint64 in big-endian format) - Absolute file offset where the next section begins
- Bytes 24-63: Additional metadata (40 bytes)

### First Section Offset

The first section offset in the snapshot header (bytes 16-23) indicates where the first data section starts. This offset is critical for reconstruction as it determines the layout of the file.

In the example snapshot (dn.snap), this offset is 192 bytes (0xC0), which means:
- 64 bytes for the main header
- 121 bytes for the header data
- 7 bytes of padding for alignment to the next 64-byte boundary

### Detailed Example: dn.snap File Structure

Let's examine the first 192 bytes of the dn.snap file to understand the structure:

```
00000000  00 01 00 00 00 00 00 00  00 00 00 00 00 00 00 79  |...............y|
00000010  00 00 00 00 00 00 00 c0  00 00 00 00 00 00 00 00  |................|
00000020  00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00  |................|
*
00000040  00 00 00 00 00 00 00 71  01 02 02 d1 08 89 a4 76  |.......q.......v|
00000050  a7 ae 43 08 75 e2 25 bf  89 84 c7 0c 93 bb b7 96  |..C.u.%...........|
00000060  54 45 94 b1 ba 58 1e 0c  8f fa b8 03 4c 01 0e 02  |TE...X......L...|
00000070  14 61 63 63 3a 2f 2f 64  6e 2e 61 63 6d 65 2f 6c  |.acc://dn.acme/l|
00000080  65 64 67 65 72 03 f7 f9  a4 11 04 ac 84 fb 83 0d  |edger...........|
00000090  08 07 09 0a 01 06 41 70  6f 6c 6c 6f 02 07 09 0f  |......Apollo....|
000000a0  01 0b 43 68 61 6e 64 72  61 79 61 61 6e 02 07 09  |..Chandrayaan...|
000000b0  08 01 04 59 75 74 75 02  07 00 00 00 00 00 00 00  |...Yutu........|
000000c0
```

Breaking this down:

1. **Main Snapshot Header** (bytes 0-63):
   - Bytes 0-1: `00 01` - Section type (1 = header)
   - Bytes 8-15: `00 00 00 00 00 00 00 79` - Header data size (121 bytes)
   - Bytes 16-23: `00 00 00 00 00 00 00 c0` - First section offset (192 bytes)

2. **Header Data** (bytes 64-184):
   - Contains the snapshot metadata (format version, etc.)
   - Starts at byte 64 and extends for 121 bytes
   - At offset 0x70 (112), we see the string "acc://dn.acme/ledger" which is part of the header data
   - The header data continues with other structured information until byte 184

3. **Padding** (bytes 185-191):
   - 7 bytes of padding to align the next section on a 64-byte boundary
   - This padding ensures that the first data section starts at offset 192 (0xC0)

4. **First Data Section** (starting at byte 192):
   - This is where the first actual data section begins
   - The offset (0xC0 = 192) is specified in the main header

### Why 192 Bytes?

The 192-byte offset (0xC0) for the first data section is not arbitrary. It represents:

- 64 bytes for the main snapshot header
- 121 bytes for the header data
- 7 bytes of padding

This structure aligns the first data section on a 64-byte boundary (192 = 3 × 64). The alignment is critical for the segmented file format, which requires sections to start on 64-byte boundaries.

The reconstruction process must preserve this exact structure, including the padding, to maintain compatibility with the original snapshot format and ensure proper alignment.

## File Organization

The implementation is split across multiple files for modularity:

1. **sc.go** - Command registration, entry point, and state management
2. **sc_parse.go** - Core parsing logic and section header processing
3. **sc_parse_records.go** - Specialized section processing functions
4. **sc_reconstruct.go** - Snapshot reconstruction functionality

## Data Structures

### In `sc.go`:

- **SectionInfo** - Tracks detailed information about a section during reconstruction
  ```go
  type SectionInfo struct {
      Type         uint32    // Section type identifier
      Order        int       // Original order in the file
      Instance     int       // Instance number for this section type
      HeaderOffset int64     // Byte offset of the section header
      DataOffset   int64     // Byte offset of the section data
      StartOffset  int64     // Byte offset of the section start
      Size         uint64    // Size of the section data in bytes
      EndOffset    int64     // Byte offset of the end of the section
  }
  ```

- **sc_State** - Main state structure for the parser
  ```go
  type sc_State struct {
      // File paths and handles
      SnapshotPath string            // Path to the snapshot file
      File         *os.File          // File handle for the snapshot
      TempDir      string            // Directory for temporary files
      SectionFiles map[uint32]*os.File // Map of section type to temporary file
      
      // Snapshot metadata
      FormatVersion     uint32          // Detected snapshot format version
      FirstSectionOffset uint64         // Offset of the first section from the header
      OriginalSections  []SectionInfo   // Sections in their original order
      
      // Summary statistics
      SectionCounts   map[uint32]int  // Count of records by section type
      SectionSizes    map[uint32]int64 // Size of each section in bytes
      TotalRecords    int             // Total number of records processed
      TotalSections   int             // Total number of sections found
      ErrorCounts     map[string]int  // Count of errors by type
      StartTime       time.Time       // When processing started
      ProcessingTime  time.Duration   // Total processing time
  }
  ```

### In `sc_parse.go`:

- **sc_SectionHeader** - Represents a section header in the snapshot file
  ```go
  type sc_SectionHeader struct {
      Type          uint16  // Section type identifier
      Size          uint64  // Size of the section data in bytes
      ContentOffset int64   // Byte offset of the section content
      NextOffset    uint64  // Byte offset of the next section
  }
  ```

## Current Functionality

The current implementation:

1. Parses snapshot files section by section
2. Writes each section's data to a separate temporary file
3. Tracks statistics about the parsing process
4. Generates a summary report

## Planned Enhancements: Test Parse Mode

### Command Line Changes

The `sc` command will be updated to support multiple input files and a `--test-parse` flag:

```
analyze sc <snap_file_1> <destination_snapfile> <snap_file_2> ... --test-parse
```

When the `--test-parse` flag is present:
- Each input snapshot is processed independently
- The destination file parameter is ignored
- For each input "x.snap", a reconstructed "x-parsed.snap" is created in the output directory
- A validation report is generated showing if reconstruction was successful

### New Functionality

#### 1. Snapshot Reconstruction

The snapshot reconstruction process must precisely replicate the original snapshot file structure, including all section headers and their exact positions. This is critical for maintaining compatibility with tools that read the snapshot format.

##### Reconstruction Algorithm

To ensure byte-for-byte identical reconstruction, the reconstruction process must follow the exact same algorithm as the original segmented writer:

1. **Write Main Header**:
   - Write the 64-byte main header with the original first section offset
   - The first section offset must match the original file (e.g., 192 bytes)

2. **Write Header Data**:
   - Write the header data immediately after the main header
   - Apply any necessary fixes to the header data (e.g., the byte at offset 0xD7)

3. **Header Data Reconstruction**:
   - The header data must be reconstructed exactly as it appears in the original file
   - The byte at offset 0xD7 (215) must be set to 0xC0 (192) to match the first section offset
   - This ensures byte-for-byte compatibility with the original snapshot format

4. **Add Padding After Header Data**:
   - Add padding bytes to reach the first section offset
   - For example, if header data is 121 bytes and first section offset is 192, add 7 bytes of padding

5. **Process Each Section in Original Order**:
   - For each section in the original file order:
     - Write the section header at the exact same offset as in the original file
     - Update the section header fields:
       - Section type: Use the original section type
       - Section size: Use the exact data size from the original section
       - Next section offset: Use the original next section offset value
     - Write the section data
     - Add padding bytes to align to the next 64-byte boundary using the formula:
       `padding_size = (64 - (current_position % 64)) % 64`

6. **Update Next Section Offsets**:
   - Ensure each section header's "next section offset" field points to the correct absolute file position of the next section
   - The last section should have a next section offset of 0

```go
// sc_ReconstructSnapshot rebuilds a snapshot file from its parsed components
func sc_ReconstructSnapshot(state *sc_State, outputPath string) error {
    // Create output file
    // Write snapshot header with the original first section offset
    // Pad to the first section offset (if needed)
    // For each section in original order:
    //   - Write section header
    //   - Copy section data from temp file
    // Update section offsets
    return nil
}
```

This function will:
- Create a new snapshot file at the specified output path
- Write the snapshot header with the original first section offset (e.g., 192 bytes)
- Preserve the exact section header structure from the original file
- Process sections in their original order (not by section type)
- Handle multiple instances of the same section type correctly
- Ensure all section headers are written at their correct offsets

### Critical Aspects of Reconstruction

1. **Preserving the First Section Offset**:
   - The first section offset in the main header (bytes 16-23) must match the original
   - For example, if the original offset is 192 bytes (0xC0), the reconstructed file must maintain this

2. **Header Data Handling**:
   - The header data must be written immediately after the main header
   - The header data size is specified in the main header (bytes 8-15)

3. **Alignment and Padding**:
   - Sections must be aligned on 64-byte boundaries
   - After writing a section's data, padding bytes (zeros) must be added if needed to reach the next 64-byte boundary
   - The formula for calculating padding: `padding_size = (64 - (current_position % 64)) % 64`
   - Padding must be added after the header data to reach the first section offset
   - For example, if header data is 121 bytes, add 7 bytes of padding to reach 192 bytes

4. **Section Order**:
   - Sections must be processed in their original order, not by section type
   - This preserves the exact layout of the original file

5. **Multiple Section Instances**:
   - Some section types may appear multiple times in the file
   - Each instance must be tracked and processed separately
   - Multiple sections of the same type are created when a section's data exceeds the maximum section size limit
   - The snapshot writer automatically creates new sections when needed based on size

### Example: Reconstructing dn.snap

For the dn.snap example with a first section offset of 192 bytes:

1. Write the main header (64 bytes)
2. Write the header data (121 bytes)
3. Add padding (7 bytes) to align to the 64-byte boundary
4. Start writing data sections at offset 192

This ensures that the reconstructed file is byte-for-byte identical to the original.

#### 2. Validation

A validation function will be added to compare original and reconstructed files:

```go
// sc_ValidateReconstruction performs byte-by-byte comparison of two snapshot files
func sc_ValidateReconstruction(originalPath, reconstructedPath string) (bool, error) {
    // Open both files
    // Compare file sizes
    // Compare content in chunks
    // Return match result and any discrepancies
    return true, nil
}
```

This function will:
- Compare file sizes to ensure they match
- Perform a byte-by-byte comparison in chunks to handle large files efficiently
- Report any discrepancies with detailed offset information

#### 3. Test Parse Workflow

The `sc_Run` function will be updated to handle the test-parse workflow:

```go
func sc_Run(cmd *cobra.Command, args []string) error {
    // Check for test-parse flag
    testParse, _ := cmd.Flags().GetBool("test-parse")
    
    if testParse {
        // For each input snapshot:
        //   - Parse into sections
        //   - Reconstruct to output file
        //   - Validate reconstruction
        //   - Report results
    } else {
        // Original functionality
    }
    
    return nil
}
```

## Modular Design Principles

The reconstruction module will be designed with these principles:

1. **Single Responsibility**: Each function has a clear, focused purpose
2. **Separation of Concerns**: Parsing, reconstruction, and validation are separate
3. **Reusability**: The reconstruction module can be used for both testing and future snapshot operations
4. **Memory Efficiency**: Continues to use streaming and temporary files to minimize memory usage

## Implementation Plan

1. Update command line argument handling to support multiple input files and the `--test-parse` flag
2. Create the `sc_reconstruct.go` file with reconstruction functionality
3. Add validation functions to `sc.go`
4. Update the `sc_Run` function to implement the test-parse workflow
5. Add reporting functions for validation results

## Multiple Sections of the Same Type

### Why Multiple Sections Exist

The snapshot format allows multiple sections of the same type (e.g., multiple type 7 record sections) to exist in a single snapshot file. This is not an error or anomaly, but a deliberate design feature. Multiple sections of the same type are created when:

1. **Section Size Limits**: When a section's data exceeds the maximum size limit, the snapshot writer automatically:
   - Closes the current section
   - Opens a new section of the same type
   - Continues writing records to the new section

2. **Data Organization**: Different groups of related records may be stored in separate sections even if they're the same type

### Handling During Reconstruction

When reconstructing snapshots, it's critical to:

1. Preserve all instances of each section type in their original order
2. Maintain the exact section structure from the original file
3. Not attempt to combine or merge sections of the same type
4. Process each section independently, even if they have the same type

### Snapshot Combination Process

When multiple snapshots are combined (e.g., using the "debug genesis ingest" command):

1. Each snapshot is processed independently
2. Records from all sections (including multiple sections of the same type) are read and added to the database
3. The original section structure is not preserved in the database - records are organized by their keys
4. When creating a new snapshot from the combined database:
   - Records are organized by type
   - New sections are created based on size limits, not based on which original snapshot the records came from
   - The new snapshot will have its own section structure that may differ from any of the input snapshots

This means that when combining snapshots:
- It's perfectly fine to combine all records of the same type from different snapshots
- The section structure of the original snapshots doesn't need to be preserved
- The new snapshot will have its own section structure based solely on the size of the data

## Future Integration with URL Hash Handling

The parser will continue to support the hybrid approach for URL hash handling:
- Using KV database for fast lookups
- Maintaining binary file format for iteration
- Avoiding loading all URLs into memory at once

This ensures that URL lookups remain efficient even with large datasets.
