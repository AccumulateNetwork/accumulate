package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type CreateKeyBook struct{}

func (CreateKeyBook) Type() types.TxType { return types.TxTypeCreateKeyBook }

func (CreateKeyBook) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	if _, ok := st.Sponsor.(*state.AdiState); !ok {
		return fmt.Errorf("invalid sponsor: want %v, got %v", types.ChainTypeAdi, st.Sponsor.Header().Type)
	}

	body := new(protocol.CreateSigSpecGroup)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	if len(body.SigSpecs) == 0 {
		return fmt.Errorf("cannot create empty sig spec group")
	}

	sgUrl, err := url.Parse(body.Url)
	if err != nil {
		return fmt.Errorf("invalid target URL: %v", err)
	}

	if !sgUrl.Identity().Equal(st.SponsorUrl) {
		return fmt.Errorf("%q does not belong to %q", sgUrl, st.SponsorUrl)
	}

	entries := make([]*protocol.SigSpec, len(body.SigSpecs))
	for i, chainId := range body.SigSpecs {
		entry := new(protocol.SigSpec)
		err = st.LoadAs(chainId, entry)
		if err != nil {
			return fmt.Errorf("failed to fetch sig spec: %v", err)
		}

		u, err := entry.ParseUrl()
		if err != nil {
			// This should not happen. Only valid URLs should be stored.
			return fmt.Errorf("invalid sig spec state: bad URL: %v", err)
		}

		if !u.Identity().Equal(st.SponsorUrl) {
			return fmt.Errorf("%q does not belong to %q", u, st.SponsorUrl)
		}

		if (entry.SigSpecId != types.Bytes32{}) {
			return fmt.Errorf("%q has already been assigned to an SSG", u)
		}

		entries[i] = entry
	}

	scc := new(protocol.SyntheticCreateChain)
	scc.Cause = types.Bytes(tx.TransactionHash()).AsBytes32()
	st.Submit(st.SponsorUrl, scc)

	ssg := protocol.NewSigSpecGroup()
	ssg.ChainUrl = types.String(sgUrl.String())

	groupChainId := types.Bytes(sgUrl.ResourceChain()).AsBytes32()
	for _, spec := range entries {
		u, err := spec.ParseUrl()
		if err != nil {
			// We already did this, so this should never fail here.
			return fmt.Errorf("invalid sig spec state: bad URL: %v", err)
		}

		specChainId := types.Bytes(u.ResourceChain()).AsBytes32()
		ssg.SigSpecs = append(ssg.SigSpecs, specChainId)
		spec.SigSpecId = groupChainId
		err = scc.Update(spec)
		if err != nil {
			return fmt.Errorf("failed to marshal state: %v", err)
		}
	}

	err = scc.Create(ssg)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %v", err)
	}

	return nil
}
