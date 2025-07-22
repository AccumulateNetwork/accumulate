- Given the prohibition on cross-domain data access, a signer cannot verify it
  is authorized to sign for an account if the signer is not local to the
  account; the account must be the one to verify that. Therefore verification
  should only happen once the account receives the authority signature. This is
  true whether the account is a transaction principal or a delegator.