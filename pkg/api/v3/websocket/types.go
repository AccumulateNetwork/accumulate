package websocket

// StreamStatus indicates the status of a sub-stream.
type StreamStatus int

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package websocket enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package websocket types.yml
