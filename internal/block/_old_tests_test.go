package block_test

func TestSyntheticTransactionsAreAlwaysRecorded(t *testing.T) {
	t.Skip("TODO Needs a receipt signature")

	exec := setupWithGenesis(t)

	// Start a block
	_, err := exec.BeginBlock(BeginBlockRequest{
		IsLeader: true,
		Height:   2,
		Time:     time.Now(),
	})
	require.NoError(t, err)

	// Create a synthetic transaction where the origin does not exist
	env := acctesting.NewTransaction().
		WithPrincipal(url.MustParse("acc://account-that-does-not-exist")).
		WithSigner(exec.Network.ValidatorPage(0), 1).
		WithCurrentTimestamp().
		WithBody(&protocol.SyntheticDepositCredits{
			SyntheticOrigin: protocol.SyntheticOrigin{Cause: [32]byte{1}},
			Amount:          1,
		}).
		InitiateSynthetic(protocol.SubnetUrl(exec.Network.LocalSubnetID)).
		Sign(protocol.SignatureTypeED25519, exec.Key)

	// Check passes
	_, perr := exec.CheckTx(env)
	if perr != nil {
		require.NoError(t, perr)
	}

	// Deliver fails
	_, perr = exec.DeliverTx(env)
	require.NotNil(t, perr)

	// Commit the block
	_, err = exec.ForceCommit()
	require.NoError(t, err)

	// Verify that the synthetic transaction was recorded
	batch := exec.DB.Begin(false)
	defer batch.Discard()
	status, err := batch.Transaction(env.GetTxHash()).GetStatus()
	require.NoError(t, err, "Failed to get the synthetic transaction status")
	require.NotZero(t, status.Code)
}

func TestExecutor_ProcessTransaction(t *testing.T) {
	exec := setupWithGenesis(t)

	// Create the keys and URLs
	alice, bob, charlie := generateKey(), generateKey(), generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	bobUrl := acctesting.AcmeLiteAddressTmPriv(bob)
	charlieUrl := acctesting.AcmeLiteAddressTmPriv(charlie)

	// Create the transaction
	envelope := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithSigner(aliceUrl, 1).
		WithCurrentTimestamp().
		WithBody(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{Url: bobUrl, Amount: *big.NewInt(1000)},
				{Url: charlieUrl, Amount: *big.NewInt(2000)},
			},
		}).
		Initiate(protocol.SignatureTypeED25519, alice)

	// Initialize the database
	batch := exec.DB.Begin(true)
	defer batch.Discard()
	txnDb := batch.Transaction(envelope.GetTxHash())
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, alice, protocol.AcmeFaucetAmount, 1e9))
	require.NoError(t, txnDb.PutSignatures(&database.SignatureSet{Signatures: envelope.Signatures}))
	require.NoError(t, batch.Commit())

	// Execute the transaction
	batch = exec.DB.Begin(true)
	defer batch.Discard()
	_, _, err := exec.ProcessTransaction(batch, envelope.Transaction)
	require.NoError(t, err)
	require.NoError(t, batch.Commit())

	// Verify the transaction was recorded
	batch = exec.DB.Begin(false)
	defer batch.Discard()
	txnDb = batch.Transaction(envelope.GetTxHash())
	_, err = txnDb.GetState()
	require.NoError(t, err)
	status, err := txnDb.GetStatus()
	require.NoError(t, err)
	require.True(t, status.Delivered)
	require.Zero(t, status.Code)
}

func TestExecutor_DeliverTx(t *testing.T) {
	exec := setupWithGenesis(t)

	// Create the keys and URLs
	alice, bob, charlie := generateKey(), generateKey(), generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	bobUrl := acctesting.AcmeLiteAddressTmPriv(bob)
	charlieUrl := acctesting.AcmeLiteAddressTmPriv(charlie)

	// Create the transaction
	envelope := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithSigner(aliceUrl, 1).
		WithCurrentTimestamp().
		WithBody(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{Url: bobUrl, Amount: *big.NewInt(1000)},
				{Url: charlieUrl, Amount: *big.NewInt(2000)},
			},
		}).
		Initiate(protocol.SignatureTypeED25519, alice)

	// Initialize the database
	batch := exec.DB.Begin(true)
	defer batch.Discard()
	txnDb := batch.Transaction(envelope.GetTxHash())
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, alice, protocol.AcmeFaucetAmount, 1e9))
	require.NoError(t, txnDb.PutSignatures(&database.SignatureSet{Signatures: envelope.Signatures}))
	require.NoError(t, batch.Commit())

	// Start a block
	_, err := exec.BeginBlock(BeginBlockRequest{
		IsLeader: true,
		Height:   2,
		Time:     time.Now(),
	})
	require.NoError(t, err)

	// Execute the transaction
	_, perr := exec.DeliverTx(envelope)
	if perr != nil {
		require.NoError(t, perr.Message)
	}

	// Commit the block
	_, err = exec.ForceCommit()
	require.NoError(t, err)

	// Verify the transaction was recorded
	batch = exec.DB.Begin(false)
	defer batch.Discard()
	txnDb = batch.Transaction(envelope.GetTxHash())
	_, err = txnDb.GetState()
	require.NoError(t, err)
	status, err := txnDb.GetStatus()
	require.NoError(t, err)
	require.True(t, status.Delivered)
	require.Zero(t, status.Code)
}
