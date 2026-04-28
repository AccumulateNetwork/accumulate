// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// APISource adapts an api.Querier2 into a Source. This is the
// production path: pipeline.Run instantiates one of these against a
// peer's v3 endpoint and feeds it to Account for every URL in the
// minimum bootstrap set.
//
// Pagination: chain enumeration and entries can run very long for
// long-lived accounts (system ledger, anchor pool). APISource
// paginates until the source signals exhaustion.
type APISource struct {
	q api.Querier2

	// PageSize is the max records requested per round trip.
	// Sub-page chunks at the v3 service level are still allowed
	// (servers can return less than requested).
	PageSize uint64
}

// NewAPISource returns an APISource over q. Default page size is 256.
func NewAPISource(q api.Querier2) *APISource {
	return &APISource{q: q, PageSize: 256}
}

func (s *APISource) Main(ctx context.Context, u *url.URL) (protocol.Account, error) {
	rec, err := s.q.QueryAccount(ctx, u, nil)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	return rec.Account, nil
}

func (s *APISource) DirectoryUrls(ctx context.Context, u *url.URL) ([]*url.URL, error) {
	var out []*url.URL
	var start uint64
	for {
		count := s.PageSize
		page, err := s.q.QueryDirectoryUrls(ctx, u, &api.DirectoryQuery{
			Range: &api.RangeOptions{Start: start, Count: &count},
		})
		if err != nil {
			return nil, fmt.Errorf("query directory range[%d:+%d]: %w", start, count, err)
		}
		if page == nil || len(page.Records) == 0 {
			break
		}
		for _, r := range page.Records {
			if r != nil && r.Value != nil {
				out = append(out, r.Value)
			}
		}
		if uint64(len(page.Records)) < count {
			break
		}
		start += uint64(len(page.Records))
	}
	return out, nil
}

func (s *APISource) PendingIDs(ctx context.Context, u *url.URL) ([]*url.TxID, error) {
	var out []*url.TxID
	var start uint64
	for {
		count := s.PageSize
		page, err := s.q.QueryPendingIds(ctx, u, &api.PendingQuery{
			Range: &api.RangeOptions{Start: start, Count: &count},
		})
		if err != nil {
			return nil, fmt.Errorf("query pending range[%d:+%d]: %w", start, count, err)
		}
		if page == nil || len(page.Records) == 0 {
			break
		}
		for _, r := range page.Records {
			if r != nil && r.Value != nil {
				out = append(out, r.Value)
			}
		}
		if uint64(len(page.Records)) < count {
			break
		}
		start += uint64(len(page.Records))
	}
	return out, nil
}

func (s *APISource) ChainNames(ctx context.Context, u *url.URL) ([]string, error) {
	var out []string
	var start uint64
	for {
		count := s.PageSize
		page, err := s.q.QueryAccountChains(ctx, u, &api.ChainQuery{
			Range: &api.RangeOptions{Start: start, Count: &count},
		})
		if err != nil {
			return nil, fmt.Errorf("query chains range[%d:+%d]: %w", start, count, err)
		}
		if page == nil || len(page.Records) == 0 {
			break
		}
		for _, r := range page.Records {
			if r != nil {
				out = append(out, r.Name)
			}
		}
		if uint64(len(page.Records)) < count {
			break
		}
		start += uint64(len(page.Records))
	}
	return out, nil
}

func (s *APISource) ChainEntries(ctx context.Context, u *url.URL, chainName string) ([][]byte, error) {
	var out [][]byte
	var start uint64
	for {
		count := s.PageSize
		expand := false
		page, err := s.q.QueryChainEntries(ctx, u, &api.ChainQuery{
			Name: chainName,
			Range: &api.RangeOptions{
				Start:  start,
				Count:  &count,
				Expand: &expand,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("query chain %s entries range[%d:+%d]: %w", chainName, start, count, err)
		}
		if page == nil || len(page.Records) == 0 {
			break
		}
		for _, r := range page.Records {
			if r == nil {
				continue
			}
			entry := make([]byte, len(r.Entry))
			copy(entry, r.Entry[:])
			out = append(out, entry)
		}
		if uint64(len(page.Records)) < count {
			break
		}
		start += uint64(len(page.Records))
	}
	return out, nil
}

// Compile-time assertion that APISource satisfies Source.
var _ Source = (*APISource)(nil)
