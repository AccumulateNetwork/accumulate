package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// Section represents a section in the snapshot file
type Section struct {
	Type    string   // Section type identifier (e.g., "1_1")
	Offset  int64    // Offset in the output file
	TmpFile *os.File // Temporary file containing section data
}

// Sections maintains an ordered list of sections
type Sections struct {
	SectionList []Section
}

// Get retrieves a section by its type string
// Returns the section pointer if found, or nil if not found
func (s *Sections) Get(typeStr string) *Section {
	for i := range s.SectionList {
		if s.SectionList[i].Type == typeStr {
			return &s.SectionList[i]
		}
	}
	return nil
}

// Add adds a new section to the Sections list
func (s *Sections) Add(typeStr string, tmpFile *os.File) {
	section := Section{
		Type:    typeStr,
		TmpFile: tmpFile,
		Offset:  0, // Will be set during reconstruction
	}
	s.SectionList = append(s.SectionList, section)
}

// List returns all sections in the order they were added
func (s *Sections) List() []Section {
	return s.SectionList
}

// Keys returns a sorted list of section type strings
func (s *Sections) Keys() []string {
	keys := make([]string, len(s.SectionList))
	for i, section := range s.SectionList {
		keys[i] = section.Type
	}
	return keys
}

// NewSections creates a new empty Sections instance
func NewSections() *Sections {
	return &Sections{
		SectionList: make([]Section, 0),
	}
}

// Close closes all section files
func (s *Sections) Close() error {
	for i := range s.SectionList {
		if s.SectionList[i].TmpFile != nil {
			s.SectionList[i].TmpFile.Close()
			s.SectionList[i].TmpFile = nil
		}
	}
	return nil
}

// Count returns the number of sections
func (s *Sections) Count() int {
	return len(s.SectionList)
}

// SortByType sorts the sections by their type string
func (s *Sections) SortByType() {
	sort.Slice(s.SectionList, func(i, j int) bool {
		return s.SectionList[i].Type < s.SectionList[j].Type
	})
}

// SortByOffset sorts the sections by their offset
func (s *Sections) SortByOffset() {
	sort.Slice(s.SectionList, func(i, j int) bool {
		return s.SectionList[i].Offset < s.SectionList[j].Offset
	})
}

// UpdateOffset updates the offset for a specific section
func (s *Sections) UpdateOffset(typeStr string, offset int64) bool {
	for i := range s.SectionList {
		if s.SectionList[i].Type == typeStr {
			s.SectionList[i].Offset = offset
			return true
		}
	}
	return false
}

// GetByIndex returns the section at the specified index
func (s *Sections) GetByIndex(index int) (*Section, bool) {
	if index < 0 || index >= len(s.SectionList) {
		return nil, false
	}
	return &s.SectionList[index], true
}

// FilterByTypePrefix returns sections whose type starts with the given prefix
func (s *Sections) FilterByTypePrefix(prefix string) []Section {
	var result []Section
	for _, section := range s.SectionList {
		if strings.HasPrefix(section.Type, prefix) {
			result = append(result, section)
		}
	}
	return result
}

// String returns a string representation of the sections
func (s *Sections) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Sections (%d):\n", len(s.SectionList)))
	for i, section := range s.SectionList {
		sb.WriteString(fmt.Sprintf("  %d: Type=%s, Offset=%d\n", i, section.Type, section.Offset))
	}
	return sb.String()
}
