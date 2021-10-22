package abci_test

import (
	"crypto/ed25519"
	"errors"
	"testing"

	testing2 "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/protocol"

	"github.com/AccumulateNetwork/accumulated/internal/abci"
	mock_abci "github.com/AccumulateNetwork/accumulated/internal/mock/abci"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
	tmabci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/libs/log"
)

func TestAccumulator(t *testing.T) {
	suite.Run(t, new(AccumulatorTestSuite))
}

func (s *AccumulatorTestSuite) TestInfo() {
	const height int64 = 10
	var rootHash = [32]byte{1, 2, 3, 4, 5, 6}

	s.State().EXPECT().BlockIndex().AnyTimes().Return(height)
	s.State().EXPECT().RootHash().AnyTimes().Return(rootHash[:])
	s.State().EXPECT().EnsureRootHash().AnyTimes().Return(rootHash[:])

	resp := s.App(nil).Info(tmabci.RequestInfo{})
	s.Require().Equal(height, resp.LastBlockHeight)
	s.Require().Equal(rootHash[:], resp.LastBlockAppHash)
}

func (s *AccumulatorTestSuite) TestQuery() {
	s.T().Skip("TODO")
}

func (s *AccumulatorTestSuite) TestInitChain() {
	s.T().Skip("TODO")
}

func (s *AccumulatorTestSuite) TestBeginBlock() {
	s.Chain().EXPECT().BeginBlock(gomock.Any())

	s.App(nil).BeginBlock(tmabci.RequestBeginBlock{})
}

func (s *AccumulatorTestSuite) TestCheckTx() {
	s.Run("Rejects TX if it fails to unmarshal", func() {
		s.Chain().EXPECT().CheckTx(gomock.Any()).Times(0)

		resp := s.App(nil).CheckTx(tmabci.RequestCheckTx{})
		s.Require().NotZero(resp.Code)
	})

	s.Run("Passes valid TX to chain", func() {
		//build a valid transaction
		_, origin, _ := ed25519.GenerateKey(nil)
		destAddress, destPrivKey, tx, err := testing2.BuildTestSynthDepositGenTx(origin)
		_ = destAddress
		_ = destPrivKey

		s.Require().NoError(err)

		data, err := tx.Marshal()
		s.Require().NoError(err)

		s.Chain().EXPECT().CheckTx(gomock.Any())

		resp := s.App(nil).CheckTx(tmabci.RequestCheckTx{Tx: data})
		s.Require().Zero(resp.Code)
	})

	s.Run("Rejects TX if CheckTx chain returns an error", func() {
		//build a valid gen transaction with an invalid tx payload
		_, origin, _ := ed25519.GenerateKey(nil)
		//make a fake destination
		destAddr := "wyleecoyote/acme"
		//make an unreasonable amount sure
		amount := uint64(1000000000)
		tx, err := testing2.BuildTestTokenTxGenTx(origin, destAddr, amount)
		//now corrupt the validation for the signature
		tx.Signature[0].Nonce = 9999999

		s.Require().NoError(err)

		data, err := tx.Marshal()
		s.Require().NoError(err)

		s.Chain().EXPECT().CheckTx(gomock.Any()).Return(errors.New("error"))

		resp := s.App(nil).CheckTx(tmabci.RequestCheckTx{Tx: data})
		s.Require().NotZero(resp.Code)
	})
}

func (s *AccumulatorTestSuite) TestDeliverTx() {
	s.Run("Rejects TX if it fails to unmarshal", func() {
		s.Chain().EXPECT().DeliverTx(gomock.Any()).Times(0)

		resp := s.App(nil).DeliverTx(tmabci.RequestDeliverTx{})
		s.Require().NotZero(resp.Code)
	})

	s.Run("Passes valid TX to chain", func() {
		//build a valid transaction
		_, origin, _ := ed25519.GenerateKey(nil)
		destAddress, destPrivKey, tx, err := testing2.BuildTestSynthDepositGenTx(origin)
		_ = destAddress
		_ = destPrivKey

		s.Require().NoError(err)

		data, err := tx.Marshal()
		s.Require().NoError(err)

		s.Chain().EXPECT().DeliverTx(gomock.Any()).Return(new(protocol.TxResult), nil)

		resp := s.App(nil).DeliverTx(tmabci.RequestDeliverTx{Tx: data})
		s.Require().Zero(resp.Code)
	})

	s.Run("Rejects TX if DeliverTx chain returns an error", func() {
		//build a valid gen transaction with an invalid tx payload
		_, origin, _ := ed25519.GenerateKey(nil)
		//make a fake destination
		destAddr := "wyleecoyote/acme"
		//make an unreasonable amount sure
		amount := uint64(1000000000)
		tx, err := testing2.BuildTestTokenTxGenTx(origin, destAddr, amount)

		data, err := tx.Marshal()
		s.Require().NoError(err)

		s.Chain().EXPECT().DeliverTx(gomock.Any()).Return(nil, errors.New("error"))

		resp := s.App(nil).DeliverTx(tmabci.RequestDeliverTx{Tx: data})
		s.Require().NotZero(resp.Code)
	})
}

func (s *AccumulatorTestSuite) TestEndBlock() {
	s.T().Skip("EndBlock does nothing")
}

func (s *AccumulatorTestSuite) TestCommit() {
	hash := []byte{1, 2, 3, 4}
	s.Chain().EXPECT().Commit().Return(hash, nil)

	resp := s.App(nil).Commit()
	s.Require().Equal(hash, resp.Data)
}

type AccumulatorTestSuite struct {
	suite.Suite
	varMap map[*testing.T]*accVars
}

type accVars struct {
	MockCtrl *gomock.Controller
	State    *mock_abci.MockState
	Chain    *mock_abci.MockChain
}

func (s *AccumulatorTestSuite) SetupSuite() {
	s.varMap = map[*testing.T]*accVars{}
}

func (s *AccumulatorTestSuite) vars() *accVars {
	v := s.varMap[s.T()]
	if v != nil {
		return v
	}

	v = new(accVars)
	v.MockCtrl = gomock.NewController(s.T())
	v.State = mock_abci.NewMockState(v.MockCtrl)
	v.Chain = mock_abci.NewMockChain(v.MockCtrl)
	s.varMap[s.T()] = v

	s.T().Cleanup(func() {
		v.MockCtrl.Finish()
	})
	return v
}

func (s *AccumulatorTestSuite) State() *mock_abci.MockState { return s.vars().State }
func (s *AccumulatorTestSuite) Chain() *mock_abci.MockChain { return s.vars().Chain }

func (s *AccumulatorTestSuite) App(addr crypto.Address) *abci.Accumulator {
	if addr == nil {
		addr = crypto.Address{}
	}

	app, err := abci.NewAccumulator(s.State(), addr, s.Chain(), log.MustNewDefaultLogger("plain", "error", false))
	s.Require().NoError(err)
	return app
}
