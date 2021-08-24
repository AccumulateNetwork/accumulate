| AIM | Title                   | Status   | Category       | Author                             | Created   |
| ----- | ----------------------- | -------- | -------------- | ---------------------------------- | --------- |
| 0     | Accumulate Fungible Token Standard | Accepted | Token Standard | Dennis Bunfield \<<dbunfield@cybitron.com>\> | 8-10-2021 |



# Summary

This document describes the functionality, data structures, and validation
rules of the **AIM-0** token standard. AIM-0 is a fungible token standard. Its
functionality is derived from the Factom Asset Token FAT-0 standard and 
is most alike the ERC-20 token standard of Ethereum.


# Motivation

The core capability of the Accumulate Network is for tokens to be issued and transacted on the network. This specification outlines 
the data structures to issue a new token. The Factom Asset Token Fungible Token Standard specification (FAT-0) serves as inspiration for
the Accumulate Token standard.

<!-- toc -->

- [Specification](#specification)
    * [Token Chain](#token-chain)
        + [Token Chain ID](#token-chain-id)
    * [Token Chain Entries](#token-chain-entries)
        + [Global Requirements](#global-requirements)
            - [On Token Chain](#on-token-chain)
            - [Strict JSON Parsing](#strict-json-parsing)
            - [External IDs and Signatures](#external-ids-and-signatures)
    * [Token Initialization Entry](#token-initialization-entry)
        + [Initialization Entry Content Example](#initialization-entry-content-example)
        + [Initialization Entry Field Summary & Validation](#initialization-entry-field-summary--validation)
        + [Signing](#signing)
    * [Token Transaction Entry](#token-transaction-entry)
        + [Transaction ID](#transaction-id)
        + [Transaction Entry Content Example](#transaction-entry-content-example)
        + [Transaction Entry JSON Field Summary & Validation](#transaction-entry-json-field-summary--validation)
        + [Signing Set](#signing-set)
        + [Addresses](#addresses)
        + [Reserved Coinbase & Burn Address](#reserved-coinbase--burn-address)
            - [Burning](#burning)
            - [Issuance](#issuance)
        + [Coinbase Transactions](#coinbase-transactions)
            - [Coinbase Signing Set](#coinbase-signing-set)
    * [Transaction Validation Requirements](#transaction-validation-requirements)
        + [T.x Requirements for all transactions](#tx-requirements-for-all-transactions)
        + [N.x Requirements for normal account-to-account transactions](#nx-requirements-for-normal-account-to-account-transactions)
        + [C.x Requirements for Coinbase distribution transactions](#cx-requirements-for-coinbase-distribution-transactions)
    * [Computing the Current State](#computing-the-current-state)
- [Implementation](#implementation)
    * [Duplicate JSON field names](#duplicate-json-field-names)
- [Copyright](#copyright)

<!-- tocstop -->

# Specification

## Identity
Tokens are issued under an ADI (i.e. identity) on the Accumulate Network. 
This identity will host the token issuance chain. A valid identity must exist on the network to
issue a token.

## Token Chain

A token chain is a managed chain that holds the state object defining a token. 
This token issuance state object includes information
on specification type, token supply, token precision, token ticker, coinbase account, circulation mode, and metadata. 
Once the token issuance state has been set, it cannot be changed.

### Coinbase Account

The identity coinbase URL must exist as a token chain prior to token issuance 
and must reside under the same identity as the token issuance. For 
tokens of unlimited supply, the identity of the controlling
coinbase URL will contain the key signing rules established via the 
identity key groups to mint and distribute new tokens. 
For tokens of limited supply, the coinbase account will receive a deposit of the 
full supply upon creation. Key groups can be leveraged to enable multi-sig for token minting 
(for unlimited supply tokens) and distribution to ensure 
no single identity has ownership or control over the coinbase token account. Thus, key groups
can make it such than an individual identity has no material claim 
on the balance held in the coinbase account.

### Token Burns
Token can be taken out of circulation by sending them back to the 
coinbase account. There are two types of configurations for burning, 
"burn" or "burn-and-mint."

Burning tokens will lower the total cap of a token in the network.  Burn-and-mint will 
replace burned tokens in the coinbase account, thus making the total available 
supply in the network fixed.

## Token States

Balances of tokens are maintained on the user accounts for a particular token type. 
Only the identity can send tokens from the token chain accounts 
owned by that identity. Key groups can be used to enable multi-sig transactions. Anyone will be allowed to make deposits into a user's account.

### Global Requirements

All valid entries must adhere to the following.

#### Strict JSON Parsing

In order to ensure consistency across implementations, all JSON must be parsed
very strictly. In addition to the JSON structure being well-formed with all
required fields present, the following are strictly prohibited:

- unknown fields
- fields with unexpected types
- duplicate field names

Note that duplicate field names are not strictly prohibited by the JSON
specification. Parsing duplicate field names may differ by implementation. See
Implementation Notes for some mechanisms to detect and prohibit duplicate field
names if the JSON library does not support their detection natively.


#### Signatures

Signature(s) are defined by [FATIP-103](103.md). The required set of
signatures depends on the key group defined for the identity.

### Token Issuance Example

```json
{
  "type": "AIM-0",
  "supply": 50000000000000000,
  "coinbase": "accumulate/coinbase",
  "mode": "burn-and-mint",
  "precision": 8, 
  "symbol": "ATK",
  "metadata": {"custom-field": "Accumulate Tokens"}
}
```

### Initialization Entry Field Summary & Validation

| Name        | Type   | Description                                                  | Validation                                                   | Required |
| ----------- | ------ | ------------------------------------------------------------ | ------------------------------------------------------------ | -------- |
| `type`      | string | The type the chain validator for the issued token.                             | Must equal 'AIM-0'.                                          | Y        |
| `coinbase`  | string | The coinbase URL account.                                    | Identity chain path for the coinbase token account             | Y        |
| `ticker`    | string | The display symbol of the token.                             | Must be A-Z,1-9, and <= 10 characters in length.             | Y        |
| `supply`    | number | The maximum possible number of tokens that can ever be issued. Expect integer units x 10^precision | Must be greater than 0, or -1 for an unlimited supply. All other values are invalid. | Y        |
| `precision` | number | The decimal accuracy of the tokens base unit (e.g. each 1 ATK is composed of 10 ^ 8 base units). | Must be an integer in the range 0-18 inclusive. Default 0.   | N        |
| `metadata`  | any    | Optional metadata defined by the Issuer.                     | This may be any valid JSON type.                             | N        |

### Signing

The Initialization Entry must be signed by the Issuer's key established by the
Identity Chain referenced in the Token Chain's Name IDs. Thus, the Identity
Chain must exist and be properly set up with a key at the time that the
Initialization Entry is submitted. See [FATIP-101](101.md) for details on key
selection from the Identity Chain and [FATIP-103](103.md) for details on
signing and External IDs.


## Token Transaction Entry

Token Transaction Entries represent the transfer of an amount of FAT-0 tokens
from one Factoid address to another. Anyone who wishes to make a transaction
must publish a valid Transaction entry to the Token Chain. There are two types
of Transactions: Normal transactions created by the holders of the Token and
Coinbase Transactions created by the Issuer.

### Transaction ID

The unique ID of a transaction is its [Factom entry
hash](https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#entry-hash).
The Transaction ID is a hash of all of the entry data including the content and
the External IDs. Entries possessing the same Transaction IDs are duplicates
and so are ignored to prevent replay attacks.

### Transaction Entry Content Example

```json
{
   "inputs:": {
      "FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC": 100,
      "FA2y6VYYPR9Y9Vyy1ZuZqWWRXGXLeuvsLWGkDxq3Ed7yc11dbBKV": 50
   },
   "outputs:": {
      "FA3aECpw3gEZ7CMQvRNxEtKBGKAos3922oqYLcHQ9NqXHudC6YBM": 150
   },
   "metadata": {"memo": "thanks for dinner!"}
}
```

### Transaction Entry JSON Field Summary & Validation

| Name      | Type   | Description                           | Validation                                                   | Required |
| --------- | ------ | ------------------------------------- | ------------------------------------------------------------ | -------- |
| `inputs`  | object | The inputs of the transaction   | Mapping of Public Factoid Address => Amount. Amount must be am integer greater than or equal to zero. May not be empty or contain duplicate Addresses. | Y        |
| `outputs` | object | The outputs of the transaction       | Mapping of Public Factoid Address => Amount. Amount must be am integer greater than or equal to zero. May not be empty or contain duplicate Addresses. May overlap with addresses found in `inputs` to form a send-to-self transaction. Sum of the Amounts must equal the sum of the `inputs` Amounts. | Y        |
| `metadata` | any    | Optional metadata defined by user        | This may be any valid JSON type.                                                     | N        |

For a Transaction to be well-formed it must follow the above defined structure
with all required fields. Additionally, the token amounts must be conserved.
In other words the sum of the inputs must be exactly equal to the sum of the
outputs. Finally duplicate addresses within a Transaction are prohibited. This
includes duplicate addresses within the inputs or outputs as well as duplicate
addresses between the inputs or outputs. Thus tokens may not be sent from an
address to itself within the same transaction. This reduces the complexity of
implementations.

### Signing Set

Transactions are signed according to [FATIP-103](103.md). The signing set is
the set of keys corresponding to the addresses in the inputs. A transaction
must include an RCD/Signature pair for each input in the transaction. The order
of the addresses in the `inputs` field does not matter, so the RCD/Signature
pairs may appear in the External IDs in any order, so long as the signatures
are properly salted with their External ID position, as specified in FATIP-103.

### Addresses

FAT-0 uses Factom's [Factoid
Address](https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#human-readable-addresses)
RCD key pairs based on ed25519 cryptography to send and receive tokens.

### Reserved Coinbase & Burn Address

The public Factoid Address
`FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC`, corresponding to the
Factoid private key with all zeros
(`Fs1KWJrpLdfucvmYwN2nWrwepLn8ercpMbzXshd1g8zyhKXLVLWj`) is a reserved address
used for two purposes: issuing and burning tokens.

#### Burning

Any tokens sent *to* the Coinbase address are forever burned and irrecoverable,
even by the Issuer of the token.

#### Issuance

Only Coinbase Transactions may use the Coinbase address as an *input*.
Coinbase Transactions issue and distribute new tokens to other addresses. To
clarify, tokens sent from this address are *not* previously burned tokens, but
new tokens adding to the circulating supply.

### Coinbase Transactions

Coinbase Transactions are special Transaction Entries generated by the Token
Issuer that issue new tokens to addresses. Coinbase Transactions may not
increase the total supply (circulating and burned) beyond the maximum supply
declared in the Initialization Entry.

Coinbase Transactions follow the same structure as normal transactions
described above, but with the Coinbase Address as the sole input in the
`inputs` object. Coinbase Transactions may not include any other address in the
`inputs` object.

#### Coinbase Signing Set

A signature from the current key establised by the Issuer's Identity key is
required. See [FATIP-101](101.md) and [FATIP-103](103.md).

## Transaction Validation Requirements

All Transactions must meet all of the T.x requirements.

Normal Transactions must additionally meet all of the N.x requirements.

Coinbase Transactions must additionally meet all of the C.x requirements.


The x.1.x requirements are generally data structure validations.

The x.2.x requirements are generally parsing and other content validations.

The x.3.x requirements are generally related to cryptographic validations.

### T.x Requirements for all transactions

- T.1.1: The content of the entry must be a single well-formed JSON.
- T.1.2: The JSON must contain all required fields listed in the above table, all fields and their members must be of the correct type. No unspecified fields may be present. No duplicate field names are allowed.
- T.1.3: A Factoid Address key may only appear once in each of the `inputs` and `outputs`
  objects. This means that the `inputs` and `outputs` may not have any duplicate keys, respectively.
- T.2.1: The sum of all amounts in the `inputs` object must be equal to the sum
  of all amounts in the `outputs` object.
- T.2.2: The entry hash of the transaction entry must be unique among all
  previously valid transactions belonging to this token.
- T.3.1: The External IDs must follow the cryptographic and structural
  requirements defined by [FATIP-103](103.md).

### N.x Requirements for normal account-to-account transactions

Normal transactions must meet all T.x and N.x requirements.

- N.2.1: The Coinbase address may not be an input.
- N.2.2: The balances of the input addresses must all be greater than or equal
  to their respective amounts declared in the transaction.
- N.3.1: For each input address, there exists a corresponding valid
  RCD/Signature pair in the External IDs as specified by
  [FATIP-103](103.md). No additional RCD/Signature pairs beyond those that
  correspond with an input may be included.

### C.x Requirements for Coinbase distribution transactions

Coinbase transactions must meet all T.x and C.x requirements.

- C.1.1: The Coinbase address must be the only input.
- C.2.1: The input amount plus the sum of tokens issued in all previous
  Coinbase transactions must be less than or equal to `supply` from the
  Issuance entry (if not unlimited).
- C.3.1: The entry must be signed by the Issuer's currently established key in
  the Identity chain according to [FATIP-101](101.md) and [FATIP-103](103.md).

## Computing the Current State

Implementations must maintain the state of the balances of all addresses in
order to evaluate the validity of a transaction. The current state can be built
by iterating through all entries in the token chain in chronological order and updating the state for any valid transaction.

The following pseudo code describes how to compute the current state of all
balances. A transaction must be applied entirely or not at all. Entries that
are not valid transactions are simply ignored. Transactions must be evaluated
in the order that they appear in the token chain. This assumes the token has
already been properly initialized.

```
for entry in token_chain.entries:
    if entry.is_valid_transaction():
        if !entry.is_coinbase_transaction():
            for input in entry.inputs:
                balances[input.address] -= input.amount
        for output in entry.outputs:
            balances[output.address] += output.amount
```

# Implementation

- [FAT Daemon & CLI](https://github.com/Factom-Asset-Tokens/fatd) (fatd) - Official FAT token daemon, API server, and CLI implementation
    - A program written in Go that tracks and validates FAT Token chains, and provides up an API for other applications to access FAT's data.
- [fat-js](https://github.com/Factom-Asset-Tokens/wallet) - Official FAT Javascript library for NodeJS & Browser
- [FAT Wallet](https://github.com/Factom-Asset-Tokens/wallet) - Official FAT Wallet UI



## Duplicate JSON field names

Most JSON libraries do not consider duplicate field names to be an error. Thus
they can go undetected if not mitigated through other detection mechanisms. One
simple way to detect duplicate field names is to compare the length of the
minified JSON with the expected length of the JSON after parsing all of the
fields. The expected length is easy to determine once all of the fields are
parsed. See the `fatd` [reference implementation](https://github.com/Factom-Asset-Tokens/fatd) of `fat0.Transaction` or
`fat0.Initialization` for examples of this.

A less efficient way to detect this is to parse the JSON, then regenerate the
JSON and compare the lengths of the JSON with the minified original.

In both cases it is necessary to minify the JSON since users may add any valid
whitespace to their JSON.


# Copyright

Copyright and related rights waived via
[CC0](https://creativecommons.org/publicdomain/zero/1.0/).