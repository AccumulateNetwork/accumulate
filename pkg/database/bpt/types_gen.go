// Code generated by gitlab.com/accumulatenetwork/core/schema. DO NOT EDIT.

package bpt

import (
	record "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/core/schema/pkg/widget"
)

type Parameters struct {
	Power           uint64
	Mask            uint64
	ArbitraryValues bool
}

var wParameters = widget.ForComposite(widget.Fields[Parameters]{
	{Name: "power", ID: 1, Widget: widget.ForUint(func(v *Parameters) *uint64 { return &v.Power })},
	{Name: "mask", ID: 2, Widget: widget.ForUint(func(v *Parameters) *uint64 { return &v.Mask })},
	{Name: "arbitraryValues", ID: 3, Widget: widget.ForBool(func(v *Parameters) *bool { return &v.ArbitraryValues })},
}, widget.Identity[*Parameters])

// Copy returns a copy of the Parameters.
func (v *Parameters) Copy() *Parameters {
	var u = new(Parameters)
	wParameters.CopyTo(u, v)
	return u
}

// EqualParameters returns true if V is equal to U.
func (v *Parameters) Equal(u *Parameters) bool {
	return wParameters.Equal(v, u)
}

type branch struct {
	Height uint64
	Key    [32]byte
	Hash   [32]byte
	bpt    *BPT
	parent *branch
	status branchStatus
	Left   node
	Right  node
}

var wbranch = widget.ForComposite(widget.Fields[branch]{
	{Name: "type", ID: 1, Widget: widget.ForTag[*nodeType]("type", (*branch).Type)},
	{Name: "height", ID: 2, Widget: widget.ForUint(func(v *branch) *uint64 { return &v.Height })},
	{Name: "key", ID: 3, Widget: widget.ForHash(func(v *branch) *[32]byte { return &v.Key })},
	{Name: "hash", ID: 4, Widget: widget.ForHash(func(v *branch) *[32]byte { return &v.Hash })},
}, widget.Identity[*branch])

func (branch) Type() nodeType { return nodeTypeBranch }

// Copy returns a copy of the branch.
func (v *branch) Copy() *branch {
	var u = new(branch)
	wbranch.CopyTo(u, v)
	return u
}

// Equalbranch returns true if V is equal to U.
func (v *branch) Equal(u *branch) bool {
	return wbranch.Equal(v, u)
}

type emptyNode struct {
	parent *branch
}

var wemptyNode = widget.ForComposite(widget.Fields[emptyNode]{
	{Name: "type", ID: 1, Widget: widget.ForTag[*nodeType]("type", (*emptyNode).Type)},
}, widget.Identity[*emptyNode])

func (emptyNode) Type() nodeType { return nodeTypeEmpty }

// Copy returns a copy of the emptyNode.
func (v *emptyNode) Copy() *emptyNode {
	var u = new(emptyNode)
	wemptyNode.CopyTo(u, v)
	return u
}

// EqualemptyNode returns true if V is equal to U.
func (v *emptyNode) Equal(u *emptyNode) bool {
	return wemptyNode.Equal(v, u)
}

type leaf struct {
	Key    *record.Key
	Value  []byte
	parent *branch
}

var wleaf = widget.ForComposite(widget.Fields[leaf]{
	{Name: "type", ID: 1, Widget: widget.ForTag[*nodeType]("type", (*leaf).Type)},
	{Name: "key", ID: 2, Widget: widget.ForValue(func(v *leaf) **record.Key { return &v.Key })},
	{Name: "value", ID: 3, Widget: widget.ForBytes(func(v *leaf) *[]byte { return &v.Value })},
}, widget.Identity[*leaf])

// Copy returns a copy of the leaf.
func (v *leaf) Copy() *leaf {
	var u = new(leaf)
	wleaf.CopyTo(u, v)
	return u
}

// Equalleaf returns true if V is equal to U.
func (v *leaf) Equal(u *leaf) bool {
	return wleaf.Equal(v, u)
}

// TODO type node interface {}

var wnode = widget.ForUnion[*nodeType](
	"type",
	node.Type,
	map[nodeType]widget.UnionMemberFields[node]{
		nodeTypeEmpty: &widget.UnionMember[node, emptyNode]{
			Fields: wemptyNode.Fields,
		},
		nodeTypeBranch: &widget.UnionMember[node, branch]{
			Fields: wbranch.Fields,
		},
		nodeTypeLeaf: &widget.UnionMember[node, leaf]{
			Fields: wleaf.Fields,
		},
	},
	widget.Identity[*node])

// copyNode returns a copy of the node.
func copyNode(v node) node {
	var u node
	wnode.CopyTo(&u, &v)
	return u
}

// equalNode returns true if A and B are equal.
func equalNode(a, b node) bool {
	return wnode.Equal(&a, &b)
}

type nodeType int64

const (
	nodeTypeBoundary            nodeType = 4
	nodeTypeBranch              nodeType = 2
	nodeTypeEmpty               nodeType = 1
	nodeTypeLeaf                nodeType = 3
	nodeTypeLeafWithExpandedKey nodeType = 5
)

var wnodeType = widget.ForEnum[*nodeType](widget.Identity[*nodeType])

// SetByName looks up a nodeType by name.
func (v *nodeType) SetByName(s string) (ok bool) {
	*v, ok = snodeType.ByName(s)
	return
}

// SetByValue looks up a nodeType by value.
func (v *nodeType) SetByValue(i int64) (ok bool) {
	*v, ok = snodeType.ByValue(i)
	return
}

// String returns the label or name of the nodeType.
func (v nodeType) String() string {
	return snodeType.String(v)
}

type stateData struct {
	RootHash  [32]byte
	MaxHeight uint64
	Parameters
}

var wstateData = widget.ForComposite(widget.Fields[stateData]{
	{Name: "rootHash", ID: 1, Widget: widget.ForHash(func(v *stateData) *[32]byte { return &v.RootHash })},
	{Name: "maxHeight", ID: 2, Widget: widget.ForUint(func(v *stateData) *uint64 { return &v.MaxHeight })},
	{ID: 3, Widget: widget.ForComposite(wParameters.Fields, func(v *stateData) *Parameters { return &v.Parameters })},
}, widget.Identity[*stateData])

// Copy returns a copy of the stateData.
func (v *stateData) Copy() *stateData {
	var u = new(stateData)
	wstateData.CopyTo(u, v)
	return u
}

// EqualstateData returns true if V is equal to U.
func (v *stateData) Equal(u *stateData) bool {
	return wstateData.Equal(v, u)
}
