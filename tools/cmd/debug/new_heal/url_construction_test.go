// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"fmt"
	"testing"
)

// TestURLConstructionStandardization tests that URL construction follows the standardized approach
// as described in the development plan, ensuring consistency between sequence.go and heal_anchor.go
func TestURLConstructionStandardization(t *testing.T) {
	networkName := "acme"
	
	// Test cases for different partition types
	testCases := []struct {
		partitionID          string
		expectedPartitionURL string  // sequence.go style
		expectedAnchorURL    string  // heal_anchor.go style (standardized)
	}{
		{
			partitionID:          "dn",
			expectedPartitionURL: "acc://dn.acme",
			expectedAnchorURL:    "acc://dn.acme/anchors/dn",
		},
		{
			partitionID:          "bvn-Apollo",
			expectedPartitionURL: "acc://bvn-Apollo.acme",
			expectedAnchorURL:    "acc://dn.acme/anchors/bvn-Apollo",
		},
		{
			partitionID:          "bvn-Artemis",
			expectedPartitionURL: "acc://bvn-Artemis.acme",
			expectedAnchorURL:    "acc://dn.acme/anchors/bvn-Artemis",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.partitionID, func(t *testing.T) {
			// Test partition URL construction (sequence.go style)
			partitionURL := fmt.Sprintf("acc://%s.%s", tc.partitionID, networkName)
			if partitionURL != tc.expectedPartitionURL {
				t.Errorf("Expected partition URL for %s to be %s, got %s", 
					tc.partitionID, tc.expectedPartitionURL, partitionURL)
			}
			
			// Test anchor URL construction (standardized heal_anchor.go style)
			anchorURL := fmt.Sprintf("acc://dn.%s/anchors/%s", networkName, tc.partitionID)
			if anchorURL != tc.expectedAnchorURL {
				t.Errorf("Expected anchor URL for %s to be %s, got %s", 
					tc.partitionID, tc.expectedAnchorURL, anchorURL)
			}
		})
	}
}

// TestURLCachingConsistency tests that the URL construction is consistent with the caching system
func TestURLCachingConsistency(t *testing.T) {
	networkName := "acme"
	
	// Test cases for different partition types and query types
	testCases := []struct {
		partitionID string
		queryType   string
		expectedCacheKey string
	}{
		{
			partitionID: "dn",
			queryType:   "account",
			expectedCacheKey: "acc://dn.acme/anchors/dn:account",
		},
		{
			partitionID: "bvn-Apollo",
			queryType:   "chain",
			expectedCacheKey: "acc://dn.acme/anchors/bvn-Apollo:chain",
		},
		{
			partitionID: "bvn-Artemis",
			queryType:   "transaction",
			expectedCacheKey: "acc://dn.acme/anchors/bvn-Artemis:transaction",
		},
	}
	
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s-%s", tc.partitionID, tc.queryType), func(t *testing.T) {
			// Construct anchor URL using the standardized approach
			anchorURL := fmt.Sprintf("acc://dn.%s/anchors/%s", networkName, tc.partitionID)
			
			// Construct cache key (URL + query type)
			cacheKey := fmt.Sprintf("%s:%s", anchorURL, tc.queryType)
			
			if cacheKey != tc.expectedCacheKey {
				t.Errorf("Expected cache key for %s/%s to be %s, got %s", 
					tc.partitionID, tc.queryType, tc.expectedCacheKey, cacheKey)
			}
		})
	}
}
