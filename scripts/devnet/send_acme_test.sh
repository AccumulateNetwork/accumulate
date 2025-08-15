#\!/bin/bash

# Generate a private key (this is deterministic for testing)
PRIVATE_KEY="70c96e83cfcc3fecb51a6ba594c31568958d202e0d9e8e1a774ac261cdf86882a9758316e646cb23e5b33bf1bbfcbb1d9e1aa7584e7099bf76bb7f93b5bb383a"
FROM_ACCOUNT="acc://a9758316e646cb23e5b33bf1bbfcbb1d9e1aa7584e7099bf/ACME"
TO_ACCOUNT="acc://a9758316e646cb23e5b33bf1bbfcbb1d9e1aa7584e7099bf/ACME"  # Send to self

echo "Sending ACME transaction..."
echo "From: $FROM_ACCOUNT"
echo "To: $TO_ACCOUNT"
echo "Amount: 1 ACME"

# Create transaction using CLI
accumulated -j --key-hex "$PRIVATE_KEY" tx create "$FROM_ACCOUNT" sendTokens --to "$TO_ACCOUNT" --amount 1 2>/dev/null || echo "Transaction failed"
