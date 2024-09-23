package wallet

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/spf13/viper"
)

func OpenAndloadWallet(reader *bufio.Reader, baseDir string) error {
	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}

	// List all available wallets
	wallets, err := listWallets()
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
	var choice int
	for {
		fmt.Print("Enter the number of the wallet you want to login to: ")
		_, err := fmt.Fscanf(reader, "%d\n", &choice)
		if err == nil && choice > 0 && choice <= len(wallets) {
			break
		} else {
			fmt.Println("Invalid choice. Please try again.")
		}
	}

	// Get the selected wallet name
	walletName := wallets[choice-1]

	// Load the wallet data
	seedPhrase, publicPass, privatePass, birthdate, err := loadWallet(walletName)
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

	err = setWalletLive(true)
	if err != nil {
		log.Printf("Error setting wallet live state: %v", err)
	}

	err = StartWallet(seedPhrase, pubPass, privPass, baseDir, walletName, birthdate, serverMode)
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
	seedPhrase, publicPass, privatePass, birthdate, err := LoadWalletAPI(walletName, password)
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

	err = setWalletLive(true)
	if err != nil {
		log.Printf("Error setting wallet live state: %v", err)
	}

	err = setWalletSync(false)
	if err != nil {
		log.Printf("Error setting wallet sync state: %v", err)
	}

	err = StartWallet(seedPhrase, pubPass, privPass, baseDir, walletName, birthdate, serverMode)
	if err != nil {
		return fmt.Errorf("failed to start wallet: %v", err)
	}

	fmt.Printf("Wallet '%s' loaded successfully.\n", walletName)
	return nil
}

// package wallet

// import (
// 	"bufio"
// 	"errors"
// 	"fmt"
// )

// func OpenAndloadWallet(reader *bufio.Reader, baseDir string) error {
// 	wallets, err := listWallets()
// 	if err != nil {
// 		return fmt.Errorf("error listing wallets: %v", err)
// 	}

// 	if len(wallets) == 0 {
// 		fmt.Println("No wallets found. Please create a new wallet first.")
// 		return errors.New("no wallets found. Please create a new wallet first")
// 	}

// 	fmt.Println("Available wallets:")
// 	for i, wallet := range wallets {
// 		fmt.Printf("%d. %s\n", i+1, wallet)
// 	}

// 	var choice int
// 	for {
// 		fmt.Print("Enter the number of the wallet you want to login to: ")
// 		_, err := fmt.Fscanf(reader, "%d\n", &choice)
// 		if err == nil && choice > 0 && choice <= len(wallets) {
// 			break
// 		} else {
// 			return errors.New("invalid choice. Please try again")
// 		}

// 	}

// 	walletName := wallets[choice-1]

// 	seedPhrase, publicPass, privatePass, birthdate, err := loadWallet(walletName)
// 	if err != nil {
// 		return fmt.Errorf("error loading wallet: %v", err)
// 	}

// 	fmt.Printf("Wallet '%s' loaded successfully.\n", walletName)

// 	pubPass := []byte(publicPass)
// 	privPass := []byte(privatePass)

// 	serverMode := true

// 	err = StartWallet(seedPhrase, pubPass, privPass, baseDir, walletName, birthdate, serverMode)
// 	if err != nil {
// 		return fmt.Errorf("failed to start wallet: %v", err)
// 	}
// 	return nil
// }
