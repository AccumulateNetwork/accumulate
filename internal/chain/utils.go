package chain

import (
	"fmt"
	"strings"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

func dnUrl() *url.URL {
	return &url.URL{Authority: "dn"}
}

func bvnUrl(subnet string) *url.URL {
	return &url.URL{Authority: "bvn-" + subnet}
}

func isBvnUrl(u *url.URL) bool {
	return u.Path == "" && strings.HasPrefix(u.Authority, "bvn-")
}

func nodeUrl(db *state.StateDB, dir bool) (*url.URL, error) {
	if dir {
		return dnUrl(), nil
	}

	subnet, err := db.SubnetID()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve subnet ID: %v", err)
	}
	return bvnUrl(subnet), nil
}

func anchorChainName(dir, major bool) string {
	parts := []string{"", "", "anchor", "pool"}

	if dir {
		parts[0] = "bvn"
	} else {
		parts[0] = "dn"
	}

	if major {
		parts[1] = "major"
	} else {
		parts[1] = "minor"
	}

	return strings.Join(parts, "-")
}
