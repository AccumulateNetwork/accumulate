package chain

import (
	"strings"

	"github.com/AccumulateNetwork/accumulate/internal/url"
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
