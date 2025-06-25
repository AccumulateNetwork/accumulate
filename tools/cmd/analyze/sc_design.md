# Snapshot Reconstruction Design Document

## Current State Analysis

### Overview
The snapshot reconstruction functionality in the Accumulate project is designed to process, combine, and reconstruct snapshot files. The current implementation has issues with exact byte-for-byte reconstruction, particularly when validating that reconstructed snapshots match the original files. The `sc` command is a streaming version of the snap-combine tool, designed to be more memory-efficient by using temporary files instead of loading entire sections into memory.

### Key Components

#### 1. File Structure
- Snapshot files begin with a 64-byte file header
- Multiple sections follow, each with:
  - 64-byte section header
  - Section data
  - Padding to maintain 64-byte alignment

#### 2. Section Types
- Header section (1_1): Contains metadata about the snapshot
- BPT section (11_x): Written directly after the header section without its own header
- Other sections (e.g., 7_1, 7_2, 8_1): Contain various types of records
  - Section 7_1: Contains account records
  - Section 7_2: Contains message records that need to be sorted when combining multiple snapshots
  - Section 8_1: Contains other types of records

#### 3. Current Processing Flow
1. `sc_Run` orchestrates the overall process:
   - Processes each input snapshot file
   - Initializes state with `scState.Init()`
   - Parses snapshots with `sc_ParseSnapshot()`
   - Combines sections from multiple snapshots
   - Reconstructs the combined snapshot with `sc_reconstruct()`
   - Validates reconstruction for single snapshot with `sc_ValidateReconstructionImpl()`

2. `sc_reconstruct` handles the reconstruction process:
   - Starts reconstruction with `sc_StartReconstruction()`
   - Writes sections to the output file with `sc_WriteSections()`
   - Updates section offsets and ensures proper alignment

3. `sc_WriteSections` writes sections to the output file:
   - Reads section headers from the original file for known sections
   - Reads section data directly from the original file or from temporary files
   - Handles special cases like the BPT section
   - Adds padding to maintain 64-byte alignment

4. `sc_ValidateReconstructionImpl` validates the reconstructed file against the original:
   - Compares file sizes
   - Performs byte-by-byte comparison
   - Prints detailed diagnostics for mismatches

### Current State of Implementation

1. **Single Snapshot Processing**:
   - Parses a single snapshot file into temporary section files
   - Reconstructs the snapshot from these temporary files
   - Validates that the reconstructed file matches the original
   - Recent fixes have improved header preservation and data reading

2. **Multiple Snapshot Processing**:
   - Basic framework exists for processing multiple snapshots
   - Combines sections from multiple snapshots into temporary files
   - Special handling for type 7 sections (accounts and messages)
   - No sorting implementation for section 7_2 (messages) yet

### Current Issues

1. **Size Mismatch**: Reconstructed files are consistently larger than the original by a few thousand bytes
2. **Header Duplication**: Suspected doubling of section headers when reading from temporary files
3. **Padding Issues**: Incorrect padding calculations affecting alignment and file size
4. **Offset Calculation**: Section offsets in reconstructed files don't match the original
5. **Memory Efficiency**: Need to ensure large snapshots can be processed without memory issues
6. **Message Sorting**: No implementation for sorting messages in section 7_2 when combining multiple snapshots

### Recent Fixes

1. **Header Preservation**: Modified code to read original section headers directly from the input file
2. **Direct Data Reading**: Updated to read section data directly from the original file for known sections
3. **Padding Calculation**: Improved padding logic to match the next section offset from the original header
4. **Alignment**: Ensured proper 64-byte alignment between sections

## Improvement Plan

### Goals

1. **Single Snapshot Reconstruction**:
   - Achieve exact byte-for-byte reconstruction for single snapshots
   - Fix all issues causing size mismatches and validation failures
   - Ensure validation passes for all test cases

2. **Multiple Snapshot Combination**:
   - Implement proper merging of records from multiple snapshots
   - Ensure correct sorting of messages in section 7_2
   - Maintain memory efficiency for large snapshots
   - Develop appropriate validation criteria for combined snapshots

### Technical Approach

#### 1. Section Header Handling
- Read section headers directly from the original file
- Update only the size and next offset fields as needed
- Maintain exact header bytes for all other fields
- For multiple snapshots, use headers from the first snapshot as templates

#### 2. Section Data Handling
- Read section data directly from the original file for single snapshots
- For multiple snapshots, combine data from temporary files
- Implement file-based bucket sort for section 7_2 messages
- Handle special cases like the BPT section consistently

#### 3. Alignment and Padding
- Ensure all sections are written at the exact offsets from the original headers
- Add padding to maintain 64-byte alignment
- Calculate padding based on the next section offset from the original header
- Ensure consistent padding approach across all section types

#### 4. Memory Efficiency
- Continue using temporary files for storing records from all snapshots
- Implement streaming processing for large sections
- Use file-based sorting algorithms for section 7_2 messages
- Avoid loading entire sections into memory at once

#### 5. Validation Strategy
- For single snapshots: Maintain exact byte-for-byte validation
- For multiple snapshots: Validate presence and correctness of all combined records
- Develop comprehensive diagnostics for validation failures
- Add validation for sorted message order in section 7_2

### Proposed Approach

#### Phase 1: Complete Single Snapshot Reconstruction Fix
1. Finalize and test the current changes for exact reconstruction of a single snapshot
2. Ensure all section headers and data are preserved exactly as in the original
3. Verify proper handling of section offsets and padding

#### Phase 2: Extend to Multiple Snapshots
1. Process all input snapshots sequentially
2. Add records from all snapshots to the same temporary files
3. Update header size fields to reflect the combined data size
4. Recalculate next section offsets based on the new sizes

#### Phase 3: Implement File-Based Sorting for Section 7_2
1. Create a file-based bucket sort implementation for messages
2. Sort messages across all input snapshots
3. Ensure memory efficiency for large datasets

#### Phase 4: Optimize Memory Usage
1. Continue using temporary files for section data
2. Process one section at a time to minimize memory footprint
3. Implement streaming processing where possible

### Technical Considerations

#### Section Header Handling
- For single snapshot reconstruction: Use exact original headers
- For multiple snapshots: Use headers from the first snapshot as templates, updating size and offset fields

#### Alignment and Padding
- Maintain strict 64-byte alignment for all sections
- Calculate padding correctly to ensure proper alignment
- For single snapshot reconstruction: Match exact padding from the original file

#### Special Section Handling
- Header Section (1_1): Always comes first
- BPT Section: Written directly after header section data without a section header
- Section 7_2 (Messages): Requires sorting when combining multiple snapshots

#### Memory Management
- Use temporary files for section data to handle large snapshots
- Implement file-based bucket sort for section 7_2
- Process one section at a time to minimize memory usage

### Validation Strategy
- For single snapshot: Expect exact byte-for-byte matching
- For multiple snapshots: Validate that all records are present and correctly sorted

## Implementation Roadmap

### Phase 1: Single Snapshot Fix (Current Focus)
1. ✅ Complete current changes to `sc_WriteSections` to preserve exact headers and data
2. ✅ Fix padding calculations to match original file offsets
3. ✅ Ensure proper handling of special sections like BPT
4. ✅ Add detailed logging for debugging section header reads and writes
5. 🔄 Add comprehensive testing for single snapshot reconstruction
6. 🔄 Verify exact byte-for-byte matching with validation

### Phase 2: Multiple Snapshot Support
1. Enhance `sc_Run` to properly combine sections from multiple snapshots
2. Update section merging logic to handle all section types correctly
3. Implement proper handling of duplicate records across snapshots
4. Update `sc_WriteSections` to handle combined data with correct offsets
5. Implement header size and offset recalculation for combined sections
6. Develop appropriate validation strategy for combined snapshots

### Phase 3: File-Based Sorting for Messages
1. Design a memory-efficient file-based bucket sort algorithm
2. Implement sorting for section 7_2 (messages) across all input snapshots
3. Ensure sorting preserves all message attributes and relationships
4. Add validation to verify correct message ordering in the output
5. Test with large datasets to confirm memory efficiency

### Phase 4: Optimization and Refinement
1. Optimize memory usage throughout the entire process
2. Enhance error handling and recovery mechanisms
3. Add performance metrics and progress reporting
4. Implement parallel processing where applicable
5. Add comprehensive logging for all processing stages
6. Develop thorough testing suite with various snapshot combinations
7. Create documentation for users and developers

## Current Status

### What's Working
1. ✅ Basic snapshot parsing and section extraction
2. ✅ Temporary file management for section data
3. ✅ Single snapshot reconstruction with preserved headers
4. ✅ Proper padding and alignment for sections
5. ✅ Special handling for BPT section
6. ✅ Basic validation for single snapshot reconstruction

### What Needs Improvement
1. 🔄 Complete validation for single snapshot reconstruction
2. ❌ Proper merging of sections from multiple snapshots
3. ❌ File-based sorting for section 7_2 messages
4. ❌ Memory optimization for large snapshots
5. ❌ Comprehensive error handling and reporting

## Open Questions and Considerations

1. **Version Compatibility**: How should we handle version differences between snapshots?
   - Should we enforce version matching?
   - Can we convert between versions if needed?
   - What fields in the header need to be updated for combined snapshots?

2. **Validation Strategy for Combined Snapshots**:
   - What criteria should be used to validate combined snapshots?
   - How can we verify that all records were properly merged?
   - What level of validation is appropriate for sorted messages?

3. **Section-Specific Processing**:
   - Are there any section types beyond 7_2 that need special handling?
   - Do any sections have dependencies that must be maintained?
   - How should we handle sections that appear in some snapshots but not others?

4. **Performance Considerations**:
   - What is the maximum expected snapshot size?
   - Are there opportunities for parallel processing?
   - Can we implement streaming processing for very large snapshots?

5. **Error Handling and Recovery**:
   - How should we handle corrupted sections in input snapshots?
   - What recovery mechanisms should be in place for failed processing?
   - How detailed should error reporting be for end users?
4. How to handle potential conflicts or duplicates when combining snapshots?
