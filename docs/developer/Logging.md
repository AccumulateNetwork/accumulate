## Logging database keys and operations

To log database operations:

- Update `const DefaultLogLevels` in `config\config.go` - add `storage=debug;` or replace with `error;storage=debug`
- Update `const debug` in `smt\storage\batch\batch.go` - enable one or more debug flags
- Update `const debugKeys` in `smt\storage\keys.go` - set to `true`

For example, `const debug = debugPut` and `const DefaultLogLevels =
"error;storage=debug"` will something like the following:

```
2022-01-13T13:44:21-06:00 DEBUG Put key=Transaction.04FD70284691414FC5E055D45B2700E6B82E7C8C606914170199A8756C2378C2 module=storage
2022-01-13T13:44:21-06:00 DEBUG Put key=Transaction.04FD70284691414FC5E055D45B2700E6B82E7C8C606914170199A8756C2378C2.State module=storage
2022-01-13T13:44:21-06:00 DEBUG Put key=Transaction.04FD70284691414FC5E055D45B2700E6B82E7C8C606914170199A8756C2378C2.Status module=storage
2022-01-13T13:44:21-06:00 DEBUG Put key=Transaction.04FD70284691414FC5E055D45B2700E6B82E7C8C606914170199A8756C2378C2.Signatures module=storage
2022-01-13T13:44:22-06:00 DEBUG Put key=Transaction.7E7C51CEB734D8CB40C4F4CA028265E6FDCC2D8443CD79EC808B60BEDDEAE39A module=storage
2022-01-13T13:44:22-06:00 DEBUG Put key=Transaction.7E7C51CEB734D8CB40C4F4CA028265E6FDCC2D8443CD79EC808B60BEDDEAE39A.State module=storage
2022-01-13T13:44:22-06:00 DEBUG Put key=Transaction.7E7C51CEB734D8CB40C4F4CA028265E6FDCC2D8443CD79EC808B60BEDDEAE39A.Status module=storage
2022-01-13T13:44:22-06:00 DEBUG Put key=Transaction.7E7C51CEB734D8CB40C4F4CA028265E6FDCC2D8443CD79EC808B60BEDDEAE39A.Signatures module=storage
2022-01-13T13:44:24-06:00 DEBUG Put key=Account.acc://foo/tokens.Chain.main.Element.1 module=storage
2022-01-13T13:44:24-06:00 DEBUG Put key=Account.acc://foo/tokens.Chain.main.Head module=storage
2022-01-13T13:44:24-06:00 DEBUG Put key=Account.acc://bvn-TestAdiAccountTx/ledger.State module=storage
2022-01-13T13:44:27-06:00 DEBUG Put key=Account.acc://foo/tokens module=storage
2022-01-13T13:44:27-06:00 DEBUG Put key=Account.acc://foo/tokens.Chain.pending.ElementIndex.7E7C51CEB734D8CB40C4F4CA028265E6FDCC2D8443CD79EC808B60BEDDEAE39A module=storage
2022-01-13T13:44:27-06:00 DEBUG Put key=Account.acc://foo/tokens.Chain.pending.Element.0 module=storage
2022-01-13T13:44:27-06:00 DEBUG Put key=Account.acc://foo/tokens.Chain.pending.Head module=storage
2022-01-13T13:44:27-06:00 DEBUG Put key=Account.acc://bvn-TestAdiAccountTx/ledger.State module=storage
```