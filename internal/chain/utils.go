package chain

import (
	"strings"

	"github.com/AccumulateNetwork/accumulate/config"
)

func anchorChainName(typ config.NetworkType, major bool) string {
	parts := []string{"", "", "anchor", "pool"}

	if typ == config.Directory {
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
