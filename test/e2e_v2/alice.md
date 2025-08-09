# Alice

Set up Alice with a basic identity: a single-sig key page and a token account.

```go
alice := build.
	Identity("alice").Create("book").
	Tokens("tokens").Create("ACME").Add(1e9).Identity().
	Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
aliceKey := alice.Book("book").Page(1).
	GenerateKey(SignatureTypeED25519)
_, _ = alice, aliceKey
```
