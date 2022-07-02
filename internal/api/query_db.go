package api

import (
	"context"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

type DatabaseQueryModule struct {
	Network *config.Describe
	DB      *database.Database
}

var _ QueryModule = (*DatabaseQueryModule)(nil)

func (m *DatabaseQueryModule) QueryState(_ context.Context, account *url.URL, fragment []string, opts QueryStateOptions) (Record, error) {
	batch := m.DB.Begin(false)
	defer batch.Discard()

	if len(fragment) > 0 {
		return nil, errors.Format(errors.StatusBadRequest, "unsupported fragment query %q", strings.Join(fragment, "/"))
	}

	return m.queryAccount(batch, account, opts)
}

func (m *DatabaseQueryModule) QuerySet(_ context.Context, account *url.URL, fragment []string, opts QuerySetOptions) (Record, error) {
	return nil, errors.Format(errors.StatusBadRequest, "unsupported fragment query %q", strings.Join(fragment, "/"))
}

func (m *DatabaseQueryModule) Search(_ context.Context, scope *url.URL, query string, opts SearchOptions) (Record, error) {
	if opts.Kind == "" {
		return nil, errors.Format(errors.StatusBadRequest, "missing option `kind`")
	}

	return nil, errors.Format(errors.StatusBadRequest, "unsupported search kind %q", opts.Kind)
}

func (m *DatabaseQueryModule) queryAccount(batch *database.Batch, accountUrl *url.URL, opts QueryStateOptions) (Record, error) {
	account := batch.Account(accountUrl)
	rec := new(AccountRecord)
	var err error
	rec.Account, err = account.GetState()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "get account %v main state: %w", accountUrl, err)
	}

	if opts.Expand {
		chains, err := account.Chains().Get()
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "get account %v chains index: %w", accountUrl, err)
		}

		for _, c := range chains {
			chain, err := account.ReadChain(c)
			if err != nil {
				return nil, errors.Format(errors.StatusUnknownError, "read account %v chain %s: %w", accountUrl, chain.Name(), err)
			}

			state := new(ChainState)
			state.Name = chain.Name()
			state.Type = chain.Type()
			state.Height = uint64(chain.Height())
			for _, hash := range chain.CurrentState().Pending {
				hash := hash // See docs/developer/rangevarref.md
				state.Roots = append(state.Roots, hash[:])
			}
			rec.Chains = append(rec.Chains, state)
		}
	}

	if opts.Prove {
		receipt := new(Receipt)
		rec.Proof = receipt
		block, mr, err := indexing.ReceiptForAccountState(m.Network, batch, account)
		if err != nil {
			receipt.Error = errors.Wrap(errors.StatusUnknownError, err).(*errors.Error)
		} else {
			receipt.LocalBlock = block
			receipt.Proof = *mr
		}
	}

	return rec, nil
}
