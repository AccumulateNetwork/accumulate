package routing

type Connector[T any] interface {
	Connect(partition string) (T, error)
}

type ConnectorFunc[T any] func(partition string) (T, error)

func (fn ConnectorFunc[T]) Connect(partition string) (T, error) { return fn(partition) }
