// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// normalizeUrl converts between different URL formats to ensure consistent routing.
// This function standardizes URLs to use the format from sequence.go (e.g., acc://bvn-Apollo.acme)
// rather than the anchor pool URL format (e.g., acc://dn.acme/anchors/Apollo).
func normalizeUrl(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}

	// If this is an anchor pool URL with a partition path, convert to partition URL
	if strings.EqualFold(u.Authority, protocol.Directory) && strings.HasPrefix(u.Path, "/anchors/") {
		parts := strings.Split(u.Path, "/")
		if len(parts) >= 3 {
			partitionID := parts[2]
			return protocol.PartitionUrl(partitionID)
		}
	}

	return u
}

// normalizeUrlsInMessage applies URL normalization to all URLs in a message
// This helps prevent routing conflicts by ensuring consistent URL formats
func normalizeUrlsInMessage(msg interface{}) {
	switch m := msg.(type) {
	case interface{ GetDestination() *url.URL }:
		if dest := m.GetDestination(); dest != nil {
			// Use type assertion to access the unexported field
			// This is a bit hacky but necessary to modify the URL
			if setter, ok := m.(interface{ SetDestination(*url.URL) }); ok {
				setter.SetDestination(normalizeUrl(dest))
			}
		}
	case interface{ GetUrl() *url.URL }:
		if u := m.GetUrl(); u != nil {
			// Use type assertion to access the unexported field
			if setter, ok := m.(interface{ SetUrl(*url.URL) }); ok {
				setter.SetUrl(normalizeUrl(u))
			}
		}
	}
}
