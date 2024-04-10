# Example notebook test

This is a simple example of a notebook-based test

### Setup

```go
import "./alice.md"
import "./bob.md"

sim := NewSim(t,
	simulator.SimpleNetwork(t.Name(), 3, 3),
	simulator.Genesis(GenesisTime).With(alice, bob),
)
```

### Send tokens from Alice to Bob

```go
st := sim.BuildAndSubmitTxnSuccessfully(
	build.Transaction().For(alice, "tokens").
		SendTokens(123, 0).To(bob, "tokens").
		SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

sim.StepUntil(
	Txn(st.TxID).Completes())
```
