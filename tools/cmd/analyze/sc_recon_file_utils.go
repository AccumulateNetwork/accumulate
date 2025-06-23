package main

import (
	"fmt"
	"os"
)

// sc_getOrCreateSectionFile is implemented in sc_parse.go

// sc_writeSnapshotHeader writes the snapshot format version header to the file
func sc_writeSnapshotHeader(file *os.File, formatVersion uint32) error {
	// Write the magic number "SNAP"
	_, err := file.Write([]byte("SNAP"))
	if err != nil {
		return fmt.Errorf("failed to write magic number: %w", err)
	}
	
	// Write the format version (4 bytes, little-endian)
	versionBytes := make([]byte, 4)
	versionBytes[0] = byte(formatVersion)
	versionBytes[1] = byte(formatVersion >> 8)
	versionBytes[2] = byte(formatVersion >> 16)
	versionBytes[3] = byte(formatVersion >> 24)
	
	_, err = file.Write(versionBytes)
	if err != nil {
		return fmt.Errorf("failed to write format version: %w", err)
	}
	
	return nil
}

// sc_writeSectionFromFile writes a section from a temporary file to the destination file
func sc_writeSectionFromFile(destFile *os.File, sourceFile *os.File, key string) error {
	// Get the file size
	fileInfo, err := sourceFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}
	
	sectionSize := fileInfo.Size()
	if sectionSize == 0 {
		// Skip empty sections
		return nil
	}
	
	// Parse the section type and instance from the key
	var sectionType uint32
	var instance int
	_, err = fmt.Sscanf(key, "%d_%d", &sectionType, &instance)
	if err != nil {
		return fmt.Errorf("failed to parse section key %s: %w", key, err)
	}
	
	// Write the section header
	// Type (4 bytes, little-endian)
	typeBytes := make([]byte, 4)
	typeBytes[0] = byte(sectionType)
	typeBytes[1] = byte(sectionType >> 8)
	typeBytes[2] = byte(sectionType >> 16)
	typeBytes[3] = byte(sectionType >> 24)
	
	_, err = destFile.Write(typeBytes)
	if err != nil {
		return fmt.Errorf("failed to write section type: %w", err)
	}
	
	// Size (8 bytes, little-endian)
	sizeBytes := make([]byte, 8)
	size := uint64(sectionSize)
	sizeBytes[0] = byte(size)
	sizeBytes[1] = byte(size >> 8)
	sizeBytes[2] = byte(size >> 16)
	sizeBytes[3] = byte(size >> 24)
	sizeBytes[4] = byte(size >> 32)
	sizeBytes[5] = byte(size >> 40)
	sizeBytes[6] = byte(size >> 48)
	sizeBytes[7] = byte(size >> 56)
	
	_, err = destFile.Write(sizeBytes)
	if err != nil {
		return fmt.Errorf("failed to write section size: %w", err)
	}
	
	// Copy the section data
	buffer := make([]byte, 1024*1024) // 1MB buffer
	remaining := sectionSize
	
	for remaining > 0 {
		// Read a chunk
		readSize := int64(len(buffer))
		if remaining < readSize {
			readSize = remaining
		}
		
		n, err := sourceFile.Read(buffer[:readSize])
		if err != nil {
			return fmt.Errorf("failed to read section data: %w", err)
		}
		
		// Write the chunk
		_, err = destFile.Write(buffer[:n])
		if err != nil {
			return fmt.Errorf("failed to write section data: %w", err)
		}
		
		remaining -= int64(n)
	}
	
	return nil
}
