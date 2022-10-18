# Accumulate CLI Wallet

The CLI lives in `./cmd/accumulate`. It can be run directly via `go run ./cmd/accumulate`, which builds to a
temporary directory and executes the binary in one go. It can be built via `go build ./cmd/accumulate`, which
creates `accumulate` or `accumulate.exe` in the current directory. It can be installed to `$GOPATH/bin/accumulate` (
GOPATH defaults to `$HOME/go`) via
`go install ./cmd/accumulate`.
