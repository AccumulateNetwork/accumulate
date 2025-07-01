package main

import (
	"encoding/hex"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// PrintMessageDetails prints detailed information about all messages found in the snapshot
func (s *ExtractState) PrintMessageDetails() {
	fmt.Printf("\n=== MESSAGE ANALYSIS ===\n")
	
	messageCount := 0
	for i, recordEntry := range s.Records {
		if recordEntry.RecordType == "message" {
			messageCount++
			fmt.Printf("\nMessage %d (Record Index: %d):\n", messageCount, i)
			
			// Print key information
			fmt.Printf("  Key Hash: %x\n", recordEntry.KeyHash)
			fmt.Printf("  Key Bytes: %s\n", hex.EncodeToString(recordEntry.Key))
			fmt.Printf("  Key String: %q\n", string(recordEntry.Key))
			
			// Analyze the key structure
			s.analyzeMessageKeyData(recordEntry.Key, messageCount)
			
			// Print value information
			fmt.Printf("  Value Size: %d bytes\n", len(recordEntry.Value))
			if len(recordEntry.Value) > 0 {
				fmt.Printf("  Value Hex (first 64 bytes): %s\n", hex.EncodeToString(recordEntry.Value[:minInt(64, len(recordEntry.Value))]))
				s.analyzeMessageValue(recordEntry.Value, messageCount)
			}
			
			// Print partition info if available
			if recordEntry.Partition != "" {
				fmt.Printf("  Partition: %s\n", recordEntry.Partition)
			}
			if recordEntry.URL != "" {
				fmt.Printf("  Account URL: %s\n", recordEntry.URL)
			}
		}
	}
	
	if messageCount == 0 {
		fmt.Printf("No messages found in the snapshot.\n")
	} else {
		fmt.Printf("\nTotal messages analyzed: %d\n", messageCount)
	}
}

// analyzeMessageKeyData analyzes the structure of a message key from raw bytes
func (s *ExtractState) analyzeMessageKeyData(keyBytes []byte, messageNum int) {
	fmt.Printf("  Key Analysis:\n")
	
	// Analyze key structure
	keyStr := string(keyBytes)
	fmt.Printf("    Key Structure Analysis:\n")
	
	// Look for common patterns
	if strings.Contains(keyStr, "Message") {
		fmt.Printf("      - Contains 'Message' identifier\n")
	}
	if strings.Contains(keyStr, "Transaction") {
		fmt.Printf("      - Contains 'Transaction' identifier\n")
	}
	
	// Look for hash patterns (32-byte sequences)
	s.findHashPatterns(keyBytes, "      ")
	
	// Look for URL patterns
	if strings.Contains(keyStr, "acc://") {
		fmt.Printf("      - Contains Accumulate URL\n")
		s.extractURLsFromKey(keyStr, "        ")
	}
	
	// Analyze key components
	s.analyzeKeyComponents(keyBytes, "      ")
}

// analyzeMessageValue analyzes the content of a message value
func (s *ExtractState) analyzeMessageValue(valueBytes []byte, messageNum int) {
	fmt.Printf("  Value Analysis:\n")
	
	// Check if it looks like JSON
	valueStr := string(valueBytes)
	if strings.HasPrefix(valueStr, "{") && strings.HasSuffix(valueStr, "}") {
		fmt.Printf("    - Appears to be JSON format\n")
		// Print first part of JSON if it's readable
		if len(valueStr) > 200 {
			fmt.Printf("    - JSON Preview: %s...\n", valueStr[:200])
		} else {
			fmt.Printf("    - JSON Content: %s\n", valueStr)
		}
	} else {
		// Analyze as binary data
		fmt.Printf("    - Binary data format\n")
		
		// Look for common patterns
		if len(valueBytes) >= 32 {
			fmt.Printf("    - Contains potential hash at start: %x\n", valueBytes[:32])
		}
		
		// Check for readable strings
		readableChars := 0
		for _, b := range valueBytes[:minInt(100, len(valueBytes))] {
			if (b >= 32 && b <= 126) || b == 9 || b == 10 || b == 13 {
				readableChars++
			}
		}
		
		readablePercent := float64(readableChars) / float64(minInt(100, len(valueBytes))) * 100
		fmt.Printf("    - Readable character percentage: %.1f%%\n", readablePercent)
		
		if readablePercent > 50 {
			// Try to show readable parts
			readable := make([]byte, 0, len(valueBytes))
			for _, b := range valueBytes {
				if (b >= 32 && b <= 126) || b == 9 || b == 10 || b == 13 {
					readable = append(readable, b)
				} else {
					readable = append(readable, '.')
				}
			}
			preview := string(readable[:minInt(200, len(readable))])
			fmt.Printf("    - Readable Preview: %q\n", preview)
		}
	}
}

// findHashPatterns looks for 32-byte hash patterns in key bytes
func (s *ExtractState) findHashPatterns(keyBytes []byte, indent string) {
	for i := 0; i <= len(keyBytes)-32; i++ {
		// Check if this looks like a hash (not all zeros, not all same byte)
		segment := keyBytes[i : i+32]
		if s.looksLikeHash(segment) {
			fmt.Printf("%s- Hash at offset %d: %x\n", indent, i, segment)
		}
	}
}

// looksLikeHash determines if a 32-byte segment looks like a hash
func (s *ExtractState) looksLikeHash(segment []byte) bool {
	if len(segment) != 32 {
		return false
	}
	
	// Check for all zeros
	allZeros := true
	for _, b := range segment {
		if b != 0 {
			allZeros = false
			break
		}
	}
	if allZeros {
		return false
	}
	
	// Check for all same byte
	firstByte := segment[0]
	allSame := true
	for _, b := range segment {
		if b != firstByte {
			allSame = false
			break
		}
	}
	if allSame {
		return false
	}
	
	return true
}

// extractURLsFromKey extracts and analyzes URLs found in key strings
func (s *ExtractState) extractURLsFromKey(keyStr, indent string) {
	// Find all acc:// URLs
	start := 0
	for {
		idx := strings.Index(keyStr[start:], "acc://")
		if idx == -1 {
			break
		}
		
		urlStart := start + idx
		urlEnd := len(keyStr)
		
		// Find the end of the URL
		for i := urlStart; i < len(keyStr); i++ {
			if keyStr[i] == 0 || keyStr[i] == '\n' || keyStr[i] == '\r' || keyStr[i] == ' ' {
				urlEnd = i
				break
			}
		}
		
		urlStr := keyStr[urlStart:urlEnd]
		fmt.Printf("%s- URL found: %s\n", indent, urlStr)
		
		// Try to parse with Accumulate URL parser
		if accURL, err := url.Parse(urlStr); err == nil {
			fmt.Printf("%s  - Parsed successfully\n", indent)
			fmt.Printf("%s  - Authority: %s\n", indent, accURL.Authority)
			if len(accURL.Path) > 0 {
				fmt.Printf("%s  - Path: %s\n", indent, accURL.Path)
			}
		} else {
			fmt.Printf("%s  - Parse error: %v\n", indent, err)
		}
		
		start = urlEnd
	}
}

// analyzeKeyComponents analyzes the binary structure of key components
func (s *ExtractState) analyzeKeyComponents(keyBytes []byte, indent string) {
	fmt.Printf("%s- Key Components:\n", indent)
	
	// Look for length-prefixed strings (common in binary protocols)
	i := 0
	componentNum := 1
	for i < len(keyBytes) {
		if i+1 >= len(keyBytes) {
			break
		}
		
		// Check if this could be a length prefix
		length := int(keyBytes[i])
		if length > 0 && length < 100 && i+1+length <= len(keyBytes) {
			component := keyBytes[i+1 : i+1+length]
			if s.isReadableString(component) {
				fmt.Printf("%s  Component %d (len=%d): %q\n", indent, componentNum, length, string(component))
				componentNum++
				i += 1 + length
				continue
			}
		}
		
		// Check for 4-byte length prefix
		if i+4 < len(keyBytes) {
			length32 := int(keyBytes[i])<<24 | int(keyBytes[i+1])<<16 | int(keyBytes[i+2])<<8 | int(keyBytes[i+3])
			if length32 > 0 && length32 < 1000 && i+4+length32 <= len(keyBytes) {
				component := keyBytes[i+4 : i+4+length32]
				if s.isReadableString(component) {
					fmt.Printf("%s  Component %d (len32=%d): %q\n", indent, componentNum, length32, string(component))
					componentNum++
					i += 4 + length32
					continue
				}
			}
		}
		
		i++
	}
}

// isReadableString checks if a byte slice contains mostly readable characters
func (s *ExtractState) isReadableString(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	
	readableCount := 0
	for _, b := range data {
		if (b >= 32 && b <= 126) || b == 9 || b == 10 || b == 13 {
			readableCount++
		}
	}
	
	return float64(readableCount)/float64(len(data)) > 0.8
}

// minInt returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
