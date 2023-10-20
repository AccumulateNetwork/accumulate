// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dial

import (
	"fmt"
	"net"
	"sync"
)

var initIsPrivate sync.Once
var privateIPBlocks []*net.IPNet

// https://stackoverflow.com/questions/41240761/check-if-ip-address-is-in-private-network-space
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	initIsPrivate.Do(func() {
		for _, cidr := range []string{
			"127.0.0.0/8",    // IPv4 loopback
			"10.0.0.0/8",     // RFC1918
			"172.16.0.0/12",  // RFC1918
			"192.168.0.0/16", // RFC1918
			"169.254.0.0/16", // RFC3927 link-local
			"::1/128",        // IPv6 loopback
			"fe80::/10",      // IPv6 link-local
			"fc00::/7",       // IPv6 unique local addr
		} {
			_, block, err := net.ParseCIDR(cidr)
			if err != nil {
				panic(fmt.Errorf("parse error on %q: %v", cidr, err))
			}
			privateIPBlocks = append(privateIPBlocks, block)
		}
	})

	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
