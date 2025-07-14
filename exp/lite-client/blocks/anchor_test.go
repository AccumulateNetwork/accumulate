package blocks

import (
	"context"
	"errors"
	"testing"

	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	accerrors "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	messaging "gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	url "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// fakeQuerier mocks api.Querier for testing
type fakeAnchorQuerier struct {
	response interface{}
	err      error
}

func (f *fakeAnchorQuerier) Query(ctx context.Context, u *url.URL, q api.Query) (api.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	if rec, ok := f.response.(api.Record); ok {
		return rec, nil
	}
	return nil, nil
}

func TestQueryAnchorRecord_Success(t *testing.T) {
	ctx := context.Background()
	want := &api.MessageRecord[*messaging.BlockAnchor]{}
	fake := &fakeAnchorQuerier{response: want}
	got, err := QueryAnchorRecord(ctx, fake, &url.URL{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestQueryAnchorRecord_ClientError(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("client fail")
	fake := &fakeAnchorQuerier{err: sentinel}
	got, err := QueryAnchorRecord(ctx, fake, &url.URL{})
	if got != nil {
		t.Errorf("expected nil record, got %v", got)
	}
	if err != sentinel {
		t.Errorf("expected error %v, got %v", sentinel, err)
	}
}

func TestQueryAnchorRecord_NotAnchorRecord(t *testing.T) {
	ctx := context.Background()
	fake := &fakeAnchorQuerier{response: "not a BlockAnchor record"}
	_, err := QueryAnchorRecord(ctx, fake, &url.URL{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, accerrors.NotFound) {
		t.Errorf("expected NotFound error, got %v", err)
	}
}
