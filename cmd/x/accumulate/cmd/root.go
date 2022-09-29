package cmd

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/x/accumulate/walletd"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

//func GetWallet() db.DB {
//	if wallet == nil {
//		wallet = initDB(DatabaseDir, false)
//		//upon first use, make sure database format is up-to-date.
//		if !NoWalletVersionCheck {
//			out, err := RestoreAccounts()
//			if err != nil && err != db.ErrNoBucket {
//				log.Fatalf("failed to update wallet database: %v", err)
//			}
//			if out != "" {
//				log.Println("performing account database update")
//				log.Println(out)
//			}
//		}
//	}
//	return wallet
//}

var (
	Client            *client.Client
	ClientTimeout     time.Duration
	ClientDebug       bool
	WantJsonOutput    = false
	TxPretend         = false
	Prove             = false
	Memo              string
	Metadata          string
	SigType           string
	Authorities       []string
	Delegators        []string
	AdditionalSigners []string
	SignerVersion     uint
)

var currentUser = func() *user.User {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return usr
}()

var DidError error

func InitRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "accumulate",
		Short: "CLI for Accumulate Network",
	}

	cmd.SetOut(os.Stdout)

	serverAddr := os.Getenv("ACC_API")
	if serverAddr == "" {
		serverAddr = "https://testnet.accumulatenetwork.io/v2"
	}

	flags := cmd.PersistentFlags()
	flags.StringVarP(&serverAddr, "server", "s", serverAddr, "Accumulated server")
	flags.DurationVarP(&ClientTimeout, "timeout", "t", 5*time.Second, "Timeout for all API requests (i.e. 10s, 1m)")
	flags.BoolVarP(&ClientDebug, "debug", "d", false, "Print accumulated API calls")
	flags.BoolVarP(&WantJsonOutput, "json", "j", false, "print outputs as json")
	flags.BoolVarP(&TxPretend, "pretend", "n", false, "Enables check-only mode for transactions")
	flags.BoolVar(&Prove, "prove", false, "Request a receipt proving the transaction or account")
	flags.BoolVar(&TxNoWait, "no-wait", false, "Don't wait for the transaction to complete")
	flags.BoolVar(&TxIgnorePending, "ignore-pending", false, "Ignore pending transactions. Combined with --wait, this waits for transactions to be delivered.")
	flags.DurationVarP(&TxWait, "wait", "w", 0, "Wait for the transaction to complete")
	flags.StringVarP(&Memo, "memo", "m", Memo, "Memo")
	flags.StringVarP(&Metadata, "metadata", "a", Metadata, "Transaction Metadata")
	flags.StringSliceVar(&Authorities, "authority", nil, "Additional authorities to add when creating an account")
	flags.StringSliceVar(&Delegators, "delegator", nil, "Specifies the delegator when creating a delegated signature")
	flags.StringSliceVar(&AdditionalSigners, "sign-with", nil, "Specifies additional keys to sign the transaction with")
	flags.UintVar(&SignerVersion, "signer-version", uint(0), "Specify the signer version. Overrides the default behavior of fetching the signer version.")

	//TODO: to be moved to walletd configuration
	flags.UintVar(&walletd.Entropy, "entropy", uint(128), "Specifies the size of the mnemonic entropy.")
	flags.StringVar(&walletd.DatabaseDir, "database", filepath.Join(currentUser.HomeDir, ".accumulate"), "Directory the database is stored in")
	flags.BoolVar(&walletd.UseUnencryptedWallet, "use-unencrypted-wallet", false, "Use unencrypted wallet (strongly discouraged) stored at ~/.accumulate/wallet.db")
	flags.BoolVar(&walletd.NoWalletVersionCheck, "no-wallet-version-check", false, "Bypass the check to prevent updating the wallet to the format supported by the cli")

	//add the commands
	cmd.AddCommand(encryptCmd)
	cmd.AddCommand(accountCmd)
	cmd.AddCommand(adiCmd)
	cmd.AddCommand(authCmd)
	cmd.AddCommand(bookCmd)
	cmd.AddCommand(creditsCmd)
	cmd.AddCommand(dataCmd)
	cmd.AddCommand(getCmd)
	cmd.AddCommand(keyCmd)
	cmd.AddCommand(oracleCmd)
	cmd.AddCommand(pageCmd)
	cmd.AddCommand(tokenCmd)
	cmd.AddCommand(txCmd)
	cmd.AddCommand(blocksCmd)
	cmd.AddCommand(operatorCmd, validatorCmd)
	cmd.AddCommand(versionCmd, describeCmd)
	cmd.AddCommand(walletCmd)
	cmd.AddCommand(resubmitCmd)

	//for the testnet integration
	cmd.AddCommand(faucetCmd)

	cmd.PersistentPreRunE = func(*cobra.Command, []string) error {
		switch serverAddr {
		case "local":
			serverAddr = "http://127.0.1.1:26660/v2"
		case "localhost":
			serverAddr = "http://127.0.0.1:26660/v2"
		case "devnet":
			serverAddr = "https://devnet.accumulatenetwork.io/v2"
		case "testnet":
			serverAddr = "https://testnet.accumulatenetwork.io/v2"
		}

		var err error
		Client, err = client.New(serverAddr)
		if err != nil {
			return fmt.Errorf("failed to create client: %v", err)
		}
		Client.Timeout = ClientTimeout
		Client.DebugRequest = ClientDebug

		if TxNoWait {
			TxWait = 0
		}

		return nil
	}

	cmd.PersistentPostRun = func(*cobra.Command, []string) {
		if DidError != nil {
			os.Exit(1)
		}
	}

	return cmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	rootCmd := InitRootCmd()
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize()
}
