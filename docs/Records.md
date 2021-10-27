# Accumulate Data Records

- **Lite Account**
  - A token account, independent of any other record
  - Identified solely by a public key (hash) and token issuer URL
  - A lite account may only hold one type of token, which must match the account
    URL
- **ADI**
  - Accumulate Digital Identity
  - Most other record types exist as subrecords of an ADI
- **Token Issuer**
  - Defines a type of token and issues those tokens
  - Each token issuer, except those built in to the protocol, belongs to an ADI
- **ADI Token Account**
  - A token account associated with an ADI
  - An ADI token account may only hold one type of token, which is set when it
    is created
- **Key book**, previously Signature Specification Group
  - A set of key pages in order of priority
  - Belongs to an ADI
- **Key page**, previously (Multi) Signature Specification
  - A set of key specifications
  - Belongs to an ADI