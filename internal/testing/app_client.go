package testing

import (
	"context"
	"fmt"
	"sync"

	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/bytes"
	rpc "github.com/tendermint/tendermint/rpc/client"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"
)

const debugTX = false

type ABCIApplicationClient struct {
	appWg      *sync.WaitGroup
	blockMu    *sync.Mutex
	blockWg    *sync.WaitGroup
	app        abci.Application
	nextHeight func() int64
	onError    func(err error)
}

var _ rpc.ABCIClient = (*ABCIApplicationClient)(nil)

func NewABCIApplicationClient(app <-chan abci.Application, nextHeight func() int64, onError func(err error)) *ABCIApplicationClient {
	c := new(ABCIApplicationClient)
	c.appWg = new(sync.WaitGroup)
	c.blockMu = new(sync.Mutex)
	c.blockWg = new(sync.WaitGroup)
	c.nextHeight = nextHeight
	c.onError = onError

	c.appWg.Add(1)
	go func() {
		defer c.appWg.Done()
		c.app = <-app
	}()
	return c
}

func (c *ABCIApplicationClient) App() abci.Application {
	c.appWg.Wait()
	return c.app
}

// Wait until all pending transactions are done
func (c *ABCIApplicationClient) Wait() {
	c.blockWg.Wait()
}

func (c *ABCIApplicationClient) ABCIInfo(context.Context) (*ctypes.ResultABCIInfo, error) {
	r := c.App().Info(abci.RequestInfo{})
	return &ctypes.ResultABCIInfo{Response: r}, nil
}

func (c *ABCIApplicationClient) ABCIQuery(ctx context.Context, path string, data bytes.HexBytes) (*ctypes.ResultABCIQuery, error) {
	return c.ABCIQueryWithOptions(ctx, path, data, rpc.DefaultABCIQueryOptions)
}

func (c *ABCIApplicationClient) ABCIQueryWithOptions(ctx context.Context, path string, data bytes.HexBytes, opts rpc.ABCIQueryOptions) (*ctypes.ResultABCIQuery, error) {
	r := c.App().Query(abci.RequestQuery{Data: data, Path: path, Height: opts.Height, Prove: opts.Prove})
	return &ctypes.ResultABCIQuery{Response: r}, nil
}

func (c *ABCIApplicationClient) BroadcastTxAsync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	// Wait for nothing
	c.sendTx(ctx, tx)
	return &ctypes.ResultBroadcastTx{}, nil
}

func (c *ABCIApplicationClient) BroadcastTxSync(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTx, error) {
	// Wait for CheckTx
	checkChan, _ := c.sendTx(ctx, tx)
	cr := <-checkChan
	return &ctypes.ResultBroadcastTx{
		Code:         cr.Code,
		Data:         cr.Data,
		Log:          cr.Log,
		Codespace:    cr.Codespace,
		MempoolError: cr.MempoolError,
		// TODO Hash
	}, nil
}

func (c *ABCIApplicationClient) BroadcastTxCommit(ctx context.Context, tx types.Tx) (*ctypes.ResultBroadcastTxCommit, error) {
	checkChan, deliverChan := c.sendTx(ctx, tx)
	cr := <-checkChan
	dr := <-deliverChan
	return &ctypes.ResultBroadcastTxCommit{
		CheckTx:   cr,
		DeliverTx: dr,
		// TODO Hash, Height
	}, nil
}

func (c *ABCIApplicationClient) enterBlock() {
	app := c.App()
	c.blockMu.Lock()
	begin := abci.RequestBeginBlock{}
	begin.Header.Height = c.nextHeight()
	app.BeginBlock(begin)
}

func (c *ABCIApplicationClient) exitBlock() {
	app := c.App()
	app.EndBlock(abci.RequestEndBlock{})

	// Should commit always happen? Or only on success?
	app.Commit()

	c.blockMu.Unlock()
}

func (c *ABCIApplicationClient) sendTx(ctx context.Context, tx types.Tx) (<-chan abci.ResponseCheckTx, <-chan abci.ResponseDeliverTx) {
	app := c.App()
	checkChan := make(chan abci.ResponseCheckTx, 1)
	deliverChan := make(chan abci.ResponseDeliverTx, 1)

	gtx := new(transactions.GenTransaction)
	if debugTX {
		_, err := gtx.UnMarshal(tx)
		if err != nil {
			// This should never be enabled outside of debugging, so panicking
			// is borderline acceptable
			panic(err)
		}
		fmt.Printf("Got TX %X\n", gtx.TransactionHash())
	}

	c.blockWg.Add(1)
	go func() {
		defer c.blockWg.Done()

		defer close(checkChan)
		defer close(deliverChan)
		if debugTX {
			defer fmt.Printf("Finished TX %X\n", gtx.TxHash)
		}

		// TODO Roll up multiple TXs into one block?
		c.enterBlock()
		defer c.exitBlock()

		rc := app.CheckTx(abci.RequestCheckTx{Tx: tx})
		checkChan <- rc
		if rc.Code != 0 {
			c.onError(fmt.Errorf("CheckTx failed: %v\n", rc.Info))
			return
		}

		rd := app.DeliverTx(abci.RequestDeliverTx{Tx: tx})
		deliverChan <- rd
		if rd.Code != 0 {
			c.onError(fmt.Errorf("DeliverTx failed: %v\n", rd.Info))
			return
		}
	}()

	return checkChan, deliverChan
}

func (c *ABCIApplicationClient) Batch(inBlock func(func(*transactions.GenTransaction))) {
	app := c.App()

	c.blockWg.Add(1)
	defer c.blockWg.Done()

	c.enterBlock()
	defer c.exitBlock()

	inBlock(func(gtx *transactions.GenTransaction) {
		tx, err := gtx.Marshal()
		if err != nil {
			c.onError(err)
			return
		}

		rc := app.CheckTx(abci.RequestCheckTx{Tx: tx})
		if rc.Code != 0 {
			c.onError(fmt.Errorf("CheckTx failed: %v\n", rc.Info))
			return
		}

		rd := app.DeliverTx(abci.RequestDeliverTx{Tx: tx})
		if rd.Code != 0 {
			c.onError(fmt.Errorf("DeliverTx failed: %v\n", rd.Info))
			return
		}
	})
}
