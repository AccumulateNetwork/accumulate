# Bob

Set up Bob with a basic identity: a single-sig key page and a token account.

```go
bob := build.
	Identity("bob").Create("book").
	Tokens("tokens").Create("ACME").Add(1e9).Identity().
	Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
bobKey := bob.Book("book").Page(1).
	GenerateKey(SignatureTypeED25519)
_, _ = bob, bobKey
```
