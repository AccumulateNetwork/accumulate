package api

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type router interface {
	RouteAccount(*url.URL) (string, error)
	Route(...*protocol.Envelope) (string, error)
}

func ClientsMap[T any](clients map[string]T) func(string) (T, error) {
	return func(partition string) (T, error) {
		var zero T
		client, ok := clients[partition]
		if !ok {
			return zero, errors.Format(errors.StatusInternalError, "no client defined for partition %q", partition)
		}
		return client, nil
	}
}

type RoutedQueryLayer struct {
	Clients func(string) (QueryModule, error)
	Router  router
}

func (r *RoutedQueryLayer) route(account *url.URL) (QueryModule, error) {
	partition, err := r.Router.RouteAccount(account)
	if err != nil {
		return nil, err
	}

	client, err := r.Clients(partition)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (r *RoutedQueryLayer) QueryState(ctx context.Context, account *url.URL, fragment []string, opts QueryStateOptions) (Record, error) {
	client, err := r.route(account)
	if err != nil {
		return nil, err
	}
	return client.QueryState(ctx, account, fragment, opts)
}

func (r *RoutedQueryLayer) QuerySet(ctx context.Context, account *url.URL, fragment []string, opts QuerySetOptions) (Record, error) {
	client, err := r.route(account)
	if err != nil {
		return nil, err
	}
	return client.QuerySet(ctx, account, fragment, opts)
}

func (r *RoutedQueryLayer) Search(ctx context.Context, scope *url.URL, query string, opts SearchOptions) (Record, error) {
	client, err := r.route(scope)
	if err != nil {
		return nil, err
	}
	return client.Search(ctx, scope, query, opts)
}

type RoutedSubmitLayer struct {
	Clients func(string) (SubmitModule, error)
	Router  router
}

func (r *RoutedSubmitLayer) Submit(ctx context.Context, envelope *protocol.Envelope, opts SubmitOptions) (*Submission, error) {
	partition, err := r.Router.Route(envelope)
	if err != nil {
		return nil, err
	}

	client, err := r.Clients(partition)
	if err != nil {
		return nil, err
	}

	return client.Submit(ctx, envelope, opts)
}
