# Write Data

### Setup

```go
import "./alice.md"
import "./bob.md"

sim := NewSim(t,
	simulator.SimpleNetwork(t.Name(), 3, 3),
	simulator.Genesis(GenesisTime).With(alice, bob),
)
```

## Proxy data entries
### Simple entry

Submit the transaction

```go
const foo, bar = "foo", "bar"
st := sim.BuildAndSubmitTxnSuccessfully(
	build.Transaction().For(alice, "data").
		WriteData().Proxy(foo, bar).
		SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

sim.StepUntil(
	Txn(st.TxID).Completes())
```

Verify the content is replaced by hashes, and verify the hashes are correct

```go
txn := sim.QueryTransaction(st.TxID, nil).Message.Transaction
require.Equal(t, st.TxID.Hash(), txn.Hash())

require.IsType(t, txn.Body, (*WriteData)(nil))
require.IsType(t, txn.Body.(*WriteData).Entry, (*ProxyDataEntry)(nil))
entry := txn.Body.(*WriteData).Entry.(*ProxyDataEntry)

require.Nil(t, entry.Data)
require.Equal(t, [][32]byte{
	sha256.Sum256([]byte(foo)),
	sha256.Sum256([]byte(bar)),
}, entry.Hashes)
```
