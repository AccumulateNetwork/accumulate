## Chain Validator Design

Chain validators are organized by transaction type. The executor handles mundane tasks that are common to all chain
validators, such as authentication and authorization.

In general, every transaction requires a sponsor. Thus, the executor validates and loads the sponsor before delegating
to the chain validator. However, certain transaction types, specifically synthetic transactions that create records, may
not need an extant sponsor. The executor has a specific clause for these special cases.