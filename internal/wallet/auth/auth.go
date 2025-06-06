package auth

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/logger"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/operations"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/utils"

	"github.com/spf13/viper"
)

func OpenAndloadWallet(reader *bufio.Reader, baseDir string) error {
	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}

	// List all available wallets
	wallets, err := operations.ListWallets()
	if err != nil {
		return fmt.Errorf("error listing wallets: %v", err)
	}

	if len(wallets) == 0 {
		fmt.Println("No wallets found. Please create a new wallet first.")
		return errors.New("no wallets found. Please create a new wallet first")
	}

	// Display the available wallets
	fmt.Println("Available wallets:")
	for i, wallet := range wallets {
		fmt.Printf("%d. %s\n", i+1, wallet)
	}

	// Prompt the user to select a wallet
	fmt.Print("Enter the number of the wallet you want to login to: ")
	choiceStr, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading input: %v", err)
	}
	choiceStr = strings.TrimSpace(choiceStr)

	// Try to parse the input as an integer
	choice, err := strconv.Atoi(choiceStr)
	if err != nil {
		// Non-integer input (like a password) was entered
		fmt.Println("Invalid input: expecting a wallet number. Returning to main menu.")
		return errors.New("invalid wallet selection input")
	}

	// Check if the choice is within the valid range
	if choice <= 0 || choice > len(wallets) {
		fmt.Println("Invalid wallet number. Returning to main menu.")
		return errors.New("wallet number out of range")
	}

	// Get the selected wallet name
	walletName := wallets[choice-1]

	// Load the wallet data
	seedPhrase, publicPass, privatePass, birthdate, err := operations.LoadWallet(walletName)
	if err != nil {
		return fmt.Errorf("error loading wallet: %v", err)
	}

	var serverMode bool
	if !viper.IsSet("relay_wallet_set") {
		viper.Set("relay_wallet_set", false)
	}

	if !viper.IsSet("wallet_name") {
		viper.Set("wallet_name", "")
	}

	err = viper.WriteConfig()
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	// Check if the wallet is set to connect to the relay (i.e., "panel" mode)
	if viper.GetBool("relay_wallet_set") && viper.GetString("wallet_name") == walletName {
		log.Println("This is a relay wallet.")
		viper.Set("server_mode", true)
	}

	// If serverMode is true, ask if the user wants to use terminal mode
	if viper.GetBool("server_mode") && viper.GetString("wallet_name") == walletName {
		fmt.Print("This wallet is connected to the panel. If you use it in terminal/cli mode, it will not be connected to the panel.\nDo you want to use the wallet in terminal/CLI mode? (y/n): ")
		cliChoice, _ := reader.ReadString('\n')
		cliChoice = strings.TrimSpace(strings.ToLower(cliChoice))

		if cliChoice == "y" {
			fmt.Println("You are using the wallet in terminal mode. It will not be connected to the panel or relay.")
			viper.Set("server_mode", false)
		}
	}

	log.Println("Server mode: ", viper.GetBool("server_mode"))

	err = viper.WriteConfig()
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	// Proceed to start the wallet
	pubPass := []byte(publicPass)
	privPass := []byte(privatePass)

	if viper.IsSet("server_mode") {
		serverMode = viper.GetBool("server_mode")
	} else {
		serverMode = false
	}

	err = utils.SetWalletLive(true)
	if err != nil {
		log.Printf("Error setting wallet live state: %v", err)
	}

	err = operations.StartWallet(seedPhrase, pubPass, privPass, baseDir, walletName, birthdate, serverMode)
	if err != nil {
		return fmt.Errorf("failed to start wallet: %v", err)
	}

	fmt.Printf("Wallet '%s' loaded successfully.\n", walletName)
	return nil
}

func OpenAndLoadWalletAPI(walletName, password string, baseDir string) error {
	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}
	// Load the wallet data
	seedPhrase, publicPass, privatePass, birthdate, err := operations.LoadWalletAPI(walletName, password)
	if err != nil {
		return fmt.Errorf("error loading wallet: %v", err)
	}

	// Check if the wallet is set to connect to the relay (i.e., "panel" mode)
	var serverMode bool
	if viper.GetBool("relay_wallet_set") && viper.GetString("wallet_name") == walletName {
		serverMode = true
	} else {
		serverMode = false
	}

	// Convert string passphrases to byte slices
	pubPass := []byte(publicPass)
	privPass := []byte(privatePass)

	err = utils.SetWalletLive(true)
	if err != nil {
		log.Printf("Error setting wallet live state: %v", err)
	}

	err = utils.SetWalletSync(false)
	if err != nil {
		log.Printf("Error setting wallet sync state: %v", err)
	}

	logger.Info("Wallet opened successfully for: ", walletName)

	err = operations.StartWallet(seedPhrase, pubPass, privPass, baseDir, walletName, birthdate, serverMode)
	if err != nil {
		return fmt.Errorf("failed to start wallet: %v", err)
	}

	fmt.Printf("Wallet '%s' loaded successfully.\n", walletName)
	return nil
}
