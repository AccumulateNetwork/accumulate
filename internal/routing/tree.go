package routing

import (
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type RouteTree struct {
	overrides map[[32]byte]string
	root      prefixTreeNode
}

type prefixTreeNode interface {
	route(rn uint64, pos uint16) (string, error)
}

type prefixTreeBranch struct {
	bits     uint16
	children []prefixTreeNode
}

type prefixTreeLeaf string

func NewRouteTree(table *protocol.RoutingTable) (*RouteTree, error) {
	tree := new(RouteTree)

	// Build the override map
	tree.overrides = make(map[[32]byte]string, len(table.Overrides))
	for _, o := range table.Overrides {
		tree.overrides[o.Account.IdentityAccountID32()] = o.Partition
	}

	// Sort routes by mask then by value
	routes := table.Routes
	sort.Slice(routes, func(i, j int) bool {
		r, s := routes[i], routes[j]
		v, u := r.Value<<(64-r.Length), s.Value<<(64-s.Length)
		return v < u
	})

	// Build the prefix tree
	var err error
	tree.root, err = buildPrefixTree(routes, 0)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	return tree, nil
}

func buildPrefixTree(routes []protocol.Route, depth uint64) (prefixTreeNode, error) {
	if len(routes) == 1 {
		r := routes[0]
		if r.Length != depth {
			return nil, errors.Format(errors.StatusInternalError, "expected offset %d, got %d", depth, r.Length)
		}
		return prefixTreeLeaf(r.Partition), nil
	}

	// Get the minimum offset
	offset := routes[0].Length
	for _, r := range routes[1:] {
		if r.Length < offset {
			offset = r.Length
		}
	}

	var tree prefixTreeBranch
	var err error
	tree.bits = uint16(offset - depth)
	tree.children = make([]prefixTreeNode, 1<<tree.bits)
	mask := uint64(1<<tree.bits - 1)
	for i := range tree.children {
		n := sort.Search(len(routes), func(j int) bool {
			r := routes[j]
			v := r.Value >> (r.Length - offset)
			return v&mask > uint64(i)
		})
		if n == 0 {
			return nil, errors.Format(errors.StatusInternalError, "expected values with %b at %d:%d, found none", i, offset, depth)
		}
		tree.children[i], err = buildPrefixTree(routes[:n], offset)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
		routes = routes[n:]
	}
	return tree, err
}

func (r *RouteTree) Route(u *url.URL) (string, error) {
	s, ok := r.overrides[u.IdentityAccountID32()]
	if ok {
		return s, nil
	}

	return r.root.route(u.Routing(), 0)
}

func (b prefixTreeBranch) route(rn uint64, pos uint16) (string, error) {
	npos := pos + b.bits
	i := (rn >> uint64(64-npos)) & (1<<b.bits - 1)
	if b.children[i] == nil {
		return "", errors.Format(errors.StatusInternalError, "invalid routing table: no entry for %d at %d.%d", i, pos, b.bits)
	}

	return b.children[i].route(rn, npos)
}

func (b prefixTreeLeaf) route(_ uint64, _ uint16) (string, error) {
	return string(b), nil
}
