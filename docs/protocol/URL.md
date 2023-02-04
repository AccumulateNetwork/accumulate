# Accumulate URLs

A URL has a scheme, user info, authority, path, query, and fragment:

```
scheme://userinfo@authority/path?query#fragment
```

The scheme for Accumulate URLs must be `acc`. The `acc` scheme will be added if
no scheme is specified, but specifying some other scheme will cause the
transaction to be rejected.

In general, the user info, query, and fragment components of the URL are not
used.

The authority of a URL may include a port number, but it will be ignored for
most purposes. The hostname must not be empty. An empty authority or an
authority that only includes a port number, e.g. `:80`, will be rejected.

Accumulate is case insensitive. Two URLs that only differ in case are considered
equal. All URL components are lower-cased before they are converted into chain
IDs.

The identity chain ID is derived from the hostname:

```go
chain = sha256(lowerCase(url.Hostname))
```

The resource chain ID is derived from the hostname and the path:

```go
chain = sha256(lowerCase(url.Hostname + url.Path))
```

Routing is derived from the identity chain ID by interpreting the first 8 bytes
as a 64-bit unsigned integer (big endian):

```go
routing = bigEndianUint64(url.IdentityChain()[:8])
```

Thus, two URLs with the same authority have the same identity chain and routing,
and two URLs with the same authority and path have the same resource chain,
regardless of case.

## Token Issuers

The `ACME` token issuer is built into the protocol. In the future, other token
issuers can be created. Every token issuer has a token URL, such as `acc://ACME`
or `acc://bobs/tokens`.

## Lite Token Accounts

The steps to construct the URL of a lite token account are as follows:

1. Calculate the SHA-256 hash of the public key and hex encode the first 20 bytes.
    - For example, `C3AB8FF13720E8AD9047DD39466B3C8974E592C2`
2. Create a checksum by hashing the previous value (lower case) and hex encode the last 4 bytes.
    - `26E2A324`
4. Append the previous result to the key hash, as a checksum.
    - `acc://C3AB8FF13720E8AD9047DD39466B3C8974E592C226E2A324/bobs/tokens`

The ADI URL corresponding to the public key hash of a lite token account is
reserved as soon as any lite token account is created for that key. For example,
once `acc://deadbeef/ACME` is created as a lite token account, `acc://deadbeef`
can no longer be used.
