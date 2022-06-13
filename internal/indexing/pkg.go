package indexing

//go:generate go run ../../tools/cmd/gen-types --package indexing types.yml

func newfn[T any]() func() *T {
	return func() *T { return new(T) }
}
