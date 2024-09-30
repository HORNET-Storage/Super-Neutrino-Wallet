package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/config"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/logger"
	setupwallet "github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet"
	_ "github.com/btcsuite/btcwallet/walletdb/bdb"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

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

	logger.Init()
}

func main() {
	initConfig()
	if len(os.Args) > 1 {
		// CLI mode
		if err := rootCmd.Execute(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	} else {
		// Interactive mode
		interactiveMode()
	}
}

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
			err := setupwallet.CreateNewWallet(reader)
			if err != nil {
				log.Printf("Error setting up new wallet: %s", err)
			}
		case "2":
			err := setupwallet.ExistingWallet(reader)
			if err != nil {
				log.Printf("Error setting up wallet: %s", err)
			}
		case "3":
			err := setupwallet.OpenAndloadWallet(reader, viper.GetString("base_dir"))
			if err != nil {
				log.Printf("Error starting up wallet: %s", err)
			}
		case "4":
			fmt.Println("Deleting a wallet.")
			err := setupwallet.DeleteWallet(reader)
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
