// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package websocket

// StreamStatus indicates the status of a sub-stream.
type StreamStatus int

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package websocket enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package websocket types.yml
