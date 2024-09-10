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
)

func main() {
	log.Println("Starting Bitcoin wallet application")

	err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	baseDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Error getting current directory: %v", err)
	}

	logger.Init()

	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Println("\nBitcoin Wallet Manager")
		fmt.Println("1. Create a new wallet")
		fmt.Println("2. Import an existing wallet")
		fmt.Println("3. Login to an existing wallet")
		fmt.Println("4. Exit")
		fmt.Print("\nEnter your choice (1, 2, 3, or 4): ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			err = setupwallet.CreateNewWallet(reader)
			if err != nil {
				log.Printf("Error setting up new wallet: %s", err)
			}
		case "2":
			err = setupwallet.ExistingWallet(reader)
			if err != nil {
				log.Printf("Error setting up wallet: %s", err)
			}
		case "3":
			err = setupwallet.OpenAndloadWallet(reader, baseDir)
			if err != nil {
				log.Printf("Error starting up wallet: %s", err)
			}
		case "4":
			fmt.Println("Exiting program. Goodbye!")
			return
		default:
			fmt.Println("Invalid choice. Please try again.")
		}
	}
}
