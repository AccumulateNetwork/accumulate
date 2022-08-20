package errors

//go:generate go run ../../tools/cmd/gen-enum --package errors status.yml --omit-prefix
//go:generate go run ../../tools/cmd/gen-types --package errors error.yml

// Status is a request status code.
type Status uint64
