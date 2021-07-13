// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package factom

import "context"

// Heights contains all of the distinct heights for a factomd node and the
// Factom network.
type Heights struct {
	// The current directory block height of the local factomd node.
	DirectoryBlock uint32 `json:"directoryblockheight"`

	// The current block being worked on by the leaders in the network.
	// This block is not yet complete, but all transactions submitted will
	// go into this block (depending on network conditions, the transaction
	// may be delayed into the next block)
	Leader uint32 `json:"leaderheight"`

	// The height at which the factomd node has all the entry blocks.
	// Directory blocks are obtained first, entry blocks could be lagging
	// behind the directory block when syncing.
	EntryBlock uint32 `json:"entryblockheight"`

	// The height at which the local factomd node has all the entries. If
	// you added entries at a block height above this, they will not be
	// able to be retrieved by the local factomd until it syncs further.
	Entry uint32 `json:"entryheight"`
}

// Get uses c to call the "heights" RPC method and populates h with the result.
func (h *Heights) Get(ctx context.Context, c *Client) error {
	if err := c.FactomdRequest(ctx, "heights", nil, &h); err != nil {
		return err
	}
	return nil
}
