package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/config"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/ipc"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/logger"

	// setupwallet "github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/auth"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/creation"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/operations"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/utils"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func interactiveMode() {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("\nBitcoin Wallet Manager")
		fmt.Println("1. Create a new wallet")
		fmt.Println("2. Import an existing wallet")
		fmt.Println("3. Login to an existing wallet")
		fmt.Println("4. Delete a wallet")
		fmt.Println("5. Exit")
		fmt.Print("\nEnter your choice (1, 2, 3, 4, or 5): ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			err := creation.CreateNewWallet(reader)
			if err != nil {
				log.Printf("Error setting up new wallet: %s", err)
			}
		case "2":
			err := creation.ExistingWallet(reader)
			if err != nil {
				log.Printf("Error setting up wallet: %s", err)
			}
		case "3":
			err := auth.OpenAndloadWallet(reader, viper.GetString("base_dir"))
			if err != nil {
				log.Printf("Error starting up wallet: %s", err)
			}
		case "4":
			fmt.Println("Deleting a wallet.")
			err := operations.DeleteWallet(reader)
			if err != nil {
				log.Printf("Error deleting wallet: %s", err)
			}
		case "5":
			fmt.Println("Exiting program. Goodbye!")
			return
		default:
			fmt.Println("Invalid choice. Please try again.")
		}
	}
}

var rootCmd = &cobra.Command{
	Use:   "btc-wallet",
	Short: "Bitcoin Wallet CLI",
	Long:  `A Bitcoin wallet application with both interactive and CLI modes.`,
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.AddCommand(createWalletCmd)
	rootCmd.AddCommand(importWalletCmd)
	rootCmd.AddCommand(openWalletCmd)
	rootCmd.AddCommand(newTransactionCmd)
	rootCmd.AddCommand(rbfTransactionCmd)
	rootCmd.AddCommand(getWalletBalanceCmd)
	rootCmd.AddCommand(estimateTransactionSizeCmd)
	rootCmd.AddCommand(getTransactionHistoryCmd)
	rootCmd.AddCommand(getReceiveAddressesCmd)
	rootCmd.AddCommand(exitWalletCmd)
	rootCmd.AddCommand(deleteWalletCmd)
	rootCmd.AddCommand(viewSeedCmd)
}

func initConfig() {
	err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	err = viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}

	baseDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Error getting current directory: %v", err)
	}

	viper.Set("base_dir", baseDir)

	// Initialize the logger and ensure it starts with a fresh log file
	err = logger.Init("wallet_startup.log")
	if err != nil {
		log.Fatalf("Error initializing logger: %v", err)
	}

	// Rotate the log file to ensure each session has a clean log
	err = logger.RotateLog("wallet_startup.log")
	if err != nil {
		log.Fatalf("Error rotating log file: %v", err)
	}
}

var createWalletCmd = &cobra.Command{
	Use:   "create [wallet-name] [password] [pubKey] [apiKey]",
	Short: "Create a new wallet",
	Long: `Create a new wallet with the given name and password. 
	Optionally provide a pubKey and apiKey for panel integration.`,
	Args: cobra.RangeArgs(2, 4),
	Run: func(cmd *cobra.Command, args []string) {
		walletName := args[0]
		password := args[1]
		pubKey := ""
		apiKey := ""
		if len(args) > 2 {
			pubKey = args[2]
		}
		if len(args) > 3 {
			apiKey = args[3]
		}

		mnemonic, err := creation.CreateWalletAPI(walletName, password, pubKey, apiKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating wallet: %v\n", err)
			os.Exit(1)
		}

		result := struct {
			WalletName string `json:"walletName"`
			Mnemonic   string `json:"mnemonic"`
		}{
			WalletName: walletName,
			Mnemonic:   mnemonic,
		}

		json.NewEncoder(os.Stdout).Encode(result)
	},
}

var importWalletCmd = &cobra.Command{
	Use:   "import [wallet-name] [mnemonic] [password] [birthdate] [pubKey] [apiKey]",
	Short: "Import an existing wallet",
	Long: `Import an existing wallet with the given name, mnemonic, and password. 
	Provide the wallet's birthdate in YYYY-MM-DD format.
	Optionally provide a pubKey and apiKey for panel integration.`,
	Args: cobra.RangeArgs(4, 6),
	Run: func(cmd *cobra.Command, args []string) {
		walletName := args[0]
		mnemonic := args[1]
		password := args[2]
		birthdate := args[3]
		pubKey := ""
		apiKey := ""
		if len(args) > 4 {
			pubKey = args[4]
		}
		if len(args) > 5 {
			apiKey = args[5]
		}

		err := creation.ImportWalletAPI(walletName, mnemonic, password, birthdate, pubKey, apiKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error importing wallet: %v\n", err)
			os.Exit(1)
		}

		result := struct {
			WalletName string `json:"walletName"`
			Message    string `json:"message"`
		}{
			WalletName: walletName,
			Message:    "Wallet imported successfully",
		}

		json.NewEncoder(os.Stdout).Encode(result)
	},
}

var openWalletCmd = &cobra.Command{
	Use:   "open [wallet-name] [password]",
	Short: "Open and load a wallet",
	Long:  `Open and load a wallet with the given name and password.`,
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		walletName := args[0]
		password := args[1]

		baseDir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
			os.Exit(1)
		}

		logger.Info("Starting wallet open operation for: ", walletName)

		err = auth.OpenAndLoadWalletAPI(walletName, password, baseDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening wallet: %v\n", err)
			os.Exit(1)
		}

		result := struct {
			WalletName string `json:"walletName"`
			Message    string `json:"message"`
		}{
			WalletName: walletName,
			Message:    "Wallet opened successfully",
		}

		json.NewEncoder(os.Stdout).Encode(result)
	},
}

var newTransactionCmd = &cobra.Command{
	Use:   "new-transaction [recipient] [amount] [fee-rate]",
	Short: "Create a new transaction",
	Long:  `Create a new transaction with the specified recipient, amount (in satoshis), and fee rate (in sat/vB).`,
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := ipc.NewClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to wallet server: %v\n", err)
			os.Exit(1)
		}
		defer client.Close()

		result, err := client.SendCommand("new-transaction", args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error communicating with wallet server: %v\n", err)
			os.Exit(1)
		}

		// Type switch to handle different result types
		switch v := result.(type) {
		case map[string]interface{}:
			// If result is already a map, use it directly
			if errorMsg, ok := v["error"]; ok {
				fmt.Fprintf(os.Stderr, "Error creating transaction: %v\n", errorMsg)
				os.Exit(1)
			}
			json.NewEncoder(os.Stdout).Encode(v)

		case string:
			// If result is a string, try to unmarshal it
			var resultMap map[string]interface{}
			if err := json.Unmarshal([]byte(v), &resultMap); err != nil {
				fmt.Fprintf(os.Stderr, "Error unmarshaling response: %v\n", err)
				os.Exit(1)
			}
			if errorMsg, ok := resultMap["error"]; ok {
				fmt.Fprintf(os.Stderr, "Error creating transaction: %v\n", errorMsg)
				os.Exit(1)
			}
			json.NewEncoder(os.Stdout).Encode(resultMap)

		default:
			// For any other type, encode as-is
			fmt.Fprintf(os.Stderr, "Unexpected result type. Encoding as-is.\n")
			json.NewEncoder(os.Stdout).Encode(result)
		}
	},
}

var rbfTransactionCmd = &cobra.Command{
	Use:   "rbf-transaction [original-txid] [new-fee-rate]",
	Short: "Replace a transaction with a higher fee",
	Long:  `Replace an existing transaction with a new one that has a higher fee rate (in sat/vB).`,
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := ipc.NewClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to wallet server: %v\n", err)
			os.Exit(1)
		}
		defer client.Close()

		result, err := client.SendCommand("rbf-transaction", args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error communicating with wallet server: %v\n", err)
			os.Exit(1)
		}

		// For debugging
		fmt.Fprintf(os.Stderr, "Result type: %T\n", result)
		fmt.Fprintf(os.Stderr, "Result content: %+v\n", result)

		// Type switch to handle different result types
		switch v := result.(type) {
		case map[string]interface{}:
			// If result is already a map, use it directly
			if errorMsg, ok := v["error"]; ok {
				fmt.Fprintf(os.Stderr, "Error in RBF transaction: %v\n", errorMsg)
				os.Exit(1)
			}
			json.NewEncoder(os.Stdout).Encode(v)

		case string:
			// If result is a string, try to unmarshal it
			var resultMap map[string]interface{}
			if err := json.Unmarshal([]byte(v), &resultMap); err != nil {
				fmt.Fprintf(os.Stderr, "Error unmarshaling response: %v\n", err)
				os.Exit(1)
			}
			if errorMsg, ok := resultMap["error"]; ok {
				fmt.Fprintf(os.Stderr, "Error in RBF transaction: %v\n", errorMsg)
				os.Exit(1)
			}
			json.NewEncoder(os.Stdout).Encode(resultMap)

		default:
			// For any other type, encode as-is
			fmt.Fprintf(os.Stderr, "Unexpected result type. Encoding as-is.\n")
			json.NewEncoder(os.Stdout).Encode(result)
		}
	},
}

var getWalletBalanceCmd = &cobra.Command{
	Use:   "balance",
	Short: "Get the current wallet balance",
	Long:  `Retrieve the current balance of the opened wallet.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := ipc.NewClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to wallet server: %v\n", err)
			os.Exit(1)
		}
		defer client.Close()

		result, err := client.SendCommand("get-wallet-balance", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting wallet balance: %v\n", err)
			os.Exit(1)
		}
		log.Println("Wallet Balance results: ", result)

		json.NewEncoder(os.Stdout).Encode(result)
	},
}

var estimateTransactionSizeCmd = &cobra.Command{
	Use:   "estimate-tx-size [spend-amount] [recipient-address] [fee-rate]",
	Short: "Estimate transaction size",
	Long:  `Estimate the size of a transaction with the given parameters.`,
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := ipc.NewClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to wallet server: %v\n", err)
			os.Exit(1)
		}
		defer client.Close()

		spendAmount, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid spend amount: %v\n", err)
			os.Exit(1)
		}

		feeRate, err := strconv.Atoi(args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid fee rate: %v\n", err)
			os.Exit(1)
		}

		result, err := client.SendCommand("estimate-transaction-size", []string{
			strconv.FormatInt(spendAmount, 10),
			args[1], // recipient address
			strconv.Itoa(feeRate),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error estimating transaction size: %v\n", err)
			os.Exit(1)
		}

		json.NewEncoder(os.Stdout).Encode(result)
	},
}

var getTransactionHistoryCmd = &cobra.Command{
	Use:   "tx-history",
	Short: "Get transaction history",
	Long:  `Retrieve the transaction history of the opened wallet.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := ipc.NewClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to wallet server: %v\n", err)
			os.Exit(1)
		}
		defer client.Close()

		result, err := client.SendCommand("get-transaction-history", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting transaction history: %v\n", err)
			os.Exit(1)
		}

		log.Println("Transaction History: ", result)

		json.NewEncoder(os.Stdout).Encode(result)
	},
}

var getReceiveAddressesCmd = &cobra.Command{
	Use:   "get-receive-addresses",
	Short: "Get all generated receive addresses from the wallet",
	Long:  "Retrieve a list of all generated receive addresses from the wallet.",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := ipc.NewClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to wallet server: %v\n", err)
			os.Exit(1)
		}
		defer client.Close()

		result, err := client.SendCommand("get-receive-addresses", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting receive addresses: %v\n", err)
			os.Exit(1)
		}

		json.NewEncoder(os.Stdout).Encode(result)
	},
}

var exitWalletCmd = &cobra.Command{
	Use:   "exit",
	Short: "Exit and shut down the wallet",
	Long:  "Gracefully shut down the wallet.",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := ipc.NewClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to wallet server: %v\n", err)
			os.Exit(1)
		}
		defer client.Close()

		result, err := client.SendCommand("exit", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error exiting wallet: %v\n", err)
			os.Exit(1)
		}

		log.Println("Exit Result: ", result)

		json.NewEncoder(os.Stdout).Encode(result)
	},
}

var deleteWalletCmd = &cobra.Command{
	Use:   "delete [wallet-name] [password]",
	Short: "Delete an existing wallet",
	Long: `Delete an existing wallet with the given name after verifying the password.
	This action is irreversible, so proceed with caution.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		walletName := args[0]
		password := args[1]

		// Load the wallet's .env file
		walletDir := viper.GetString("wallet_dir")
		envFile := filepath.Join(walletDir, walletName+".env")
		err := godotenv.Load(envFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading wallet file: %v\n", err)
			os.Exit(1)
		}

		// Retrieve the encrypted seed phrase
		encryptedSeedPhrase := os.Getenv("ENCRYPTED_SEED_PHRASE")
		if encryptedSeedPhrase == "" {
			fmt.Fprintf(os.Stderr, "Encrypted seed phrase not found\n")
			os.Exit(1)
		}

		// Decrypt the seed phrase with the provided password
		_, err = utils.Decrypt(encryptedSeedPhrase, password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error decrypting seed phrase: incorrect password or decryption failed\n")
			os.Exit(1)
		}

		// Confirm deletion will be handled by the frontend

		// Proceed with deleting the wallet files
		err = utils.DeleteWalletFiles(walletName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting wallet files: %v\n", err)
			os.Exit(1)
		}

		err = utils.ResetLastSyncTime()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resettin Sync Time: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Wallet deleted successfully.")
	},
}

var viewSeedCmd = &cobra.Command{
	Use:   "view-seed [wallet-name] [password]",
	Short: "View the seed phrase of a wallet",
	Long:  `Retrieve and view the seed phrase of the specified wallet after verifying the password. Use this command with caution as seed phrases are sensitive information.`,
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		walletName := args[0]
		password := args[1]

		walletDir := viper.GetString("wallet_dir")
		envFile := filepath.Join(walletDir, walletName+".env")
		err := godotenv.Load(envFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading wallet file: %v\n", err)
			os.Exit(1)
		}

		encryptedSeedPhrase := os.Getenv("ENCRYPTED_SEED_PHRASE")
		if encryptedSeedPhrase == "" {
			fmt.Fprintf(os.Stderr, "Encrypted seed phrase not found\n")
			os.Exit(1)
		}

		seedPhrase, err := utils.Decrypt(encryptedSeedPhrase, password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error decrypting seed phrase: incorrect password or decryption failed\n")
			os.Exit(1)
		}

		result := struct {
			WalletName string `json:"walletName"`
			SeedPhrase string `json:"seedPhrase"`
		}{
			WalletName: walletName,
			SeedPhrase: seedPhrase,
		}

		err = json.NewEncoder(os.Stdout).Encode(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding result: %v\n", err)
			os.Exit(1)
		}
	},
}
