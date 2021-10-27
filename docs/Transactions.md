# Accumulate Transactions

- **Identity Create**
  - Create an ADI
  - Sponsored by an ADI or Lite Account
- **Token Account Create**
  - Create an ADI Token Account
  - Sponsored by an ADI
- **Token Exchange**
  - Send tokens from one account to another
  - Sponsored by a token account (ADI or Lite)
  - Recipient must be an account (ADI or Lite)
- **Create Signature Specification**
  - Create a Signature Specification
  - Sponsored by an ADI or SSG
  - When sponsored by an ADI, creates an unbound SS
  - When sponsored by an SSG, creates an SS bound to that SSG
- **Create Signature Specification Group**
  - Create a Signature Specification Group
  - Sponsored by an ADI
  - Must be created with at least one unbound Signature Specification
- **Add Credits**
  - Spend ACME tokens to add credits to a signator
  - Sponsored by a token account (ADI or Lite) holding ACME tokens
  - Recipient must be a signator (Lite Account or Signature Specification)
- **Update Key Page**
  - Add, remove, or update a key in a key page
  - Sponsored by a key page

## Synthetic

- **Synthetic Create Chain**
  - Creates chains
  - Sponsor is ignored
- **Synthetic Token Deposit**
  - Deposits tokens into an account
  - Sponsored by a token account (ADI or Lite)
  - Creates a Lite Account if the sponsor is a Lite Address with no corresponding account
- **Synthetic Deposit Credits**
  - Deposits credits into a signator
  - Sponsored by a signator (Lite Account or Signature Specification)