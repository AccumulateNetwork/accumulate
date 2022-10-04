package routing

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Connector[T any] interface {
	Connect(partition string) (T, error)
}

type ConnectorFunc[T any] func(partition string) (T, error)

func (fn ConnectorFunc[T]) Connect(partition string) (T, error) { return fn(partition) }

type QueryService struct {
	Router    *RouterInstance
	Connector Connector[api.QueryService]
}

func (s *QueryService) connect(account *url.URL) (api.QueryService, error) {
	if account == nil {
		return nil, errors.Format(errors.StatusBadRequest, "missing account parameter")
	}

	partition, err := s.Router.RouteAccount(account)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "route %s: %w", account, err)
	}

	conn, err := s.Connector.Connect(partition)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "connect to %s: %w", partition, err)
	}

	return conn, nil
}

func (s *QueryService) QueryRecord(ctx context.Context, account *url.URL, fragment []string, opts api.QueryRecordOptions) (api.Record, error) {
	c, err := s.connect(account)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	return c.QueryRecord(ctx, account, fragment, opts)
}

func (s *QueryService) QueryRange(ctx context.Context, account *url.URL, fragment []string, opts api.QueryRangeOptions) (*api.RecordRange[api.Record], error) {
	c, err := s.connect(account)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	return c.QueryRange(ctx, account, fragment, opts)
}

type SubmitService struct {
	Router    *RouterInstance
	Connector Connector[api.SubmitService]
}

func (s *SubmitService) Submit(ctx context.Context, envelope *protocol.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	if envelope == nil {
		return nil, errors.Format(errors.StatusBadRequest, "missing account parameter")
	}

	partition, err := s.Router.Route(envelope)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "route envelope: %w", err)
	}

	conn, err := s.Connector.Connect(partition)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "connect to %s: %w", partition, err)
	}

	return conn.Submit(ctx, envelope, opts)
}
