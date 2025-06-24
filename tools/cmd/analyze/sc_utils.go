package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// sc_recordError records an error and returns it
// This is a utility function that logs the error and returns it for convenience
func sc_recordError(scState *sc_State, code string, err error) error {
	// Log the error instead of storing it in a field
	fmt.Printf("ERROR [%s]: %v\n", code, err)
	return err
}

// sc_sortSectionOffsetsByPosition has been removed as part of the offset tracking cleanup

// sc_parseSectionKey parses a section key into type and index
// Renamed to avoid redeclaration with other files
func sc_parseSectionKey(key string) (int, int, error) {
	parts := strings.Split(key, "_")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid section key format: %s", key)
	}
	
	sectionType, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid section type in key %s: %w", key, err)
	}
	
	sectionIndex, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid section index in key %s: %w", key, err)
	}
	
	return sectionType, sectionIndex, nil
}

// sc_getSortedSectionKeys returns a sorted list of section keys
// Renamed to avoid redeclaration with other files
func sc_getSortedSectionKeys(sectionFiles map[string]*os.File) []string {
	keys := make([]string, 0, len(sectionFiles))
	for k := range sectionFiles {
		keys = append(keys, k)
	}
	
	// Custom sort to ensure sections are ordered by type, then by index
	sc_sortSectionKeys(keys)
	
	return keys
}

// sc_sortSectionKeys sorts section keys by type and index
func sc_sortSectionKeys(keys []string) {
	sort.Slice(keys, func(i, j int) bool {
		type1, idx1, _ := sc_parseSectionKey(keys[i])
		type2, idx2, _ := sc_parseSectionKey(keys[j])
		
		if type1 != type2 {
			return type1 < type2
		}
		return idx1 < idx2
	})
}

// sc_min returns the minimum of two integers
// This is a utility function to avoid redeclaration issues
func sc_min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// sc_max returns the maximum of two integers
// This is a utility function to avoid redeclaration issues
func sc_max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
