package creation

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/operations"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/utils"
	"github.com/spf13/viper"
	"github.com/tyler-smith/go-bip39"
	"golang.org/x/term"
)

const (
	timeFormat = "2006-01-02T15:04:05Z"
)

func CreateNewWallet(reader *bufio.Reader) error {

	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}

	fmt.Print("Enter a name for your new wallet: ")
	walletName, _ := reader.ReadString('\n')
	walletName = strings.TrimSpace(walletName)

	entropy, err := bip39.NewEntropy(256)
	if err != nil {
		return fmt.Errorf("error generating entropy: %v", err)
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return fmt.Errorf("error generating mnemonic: %v", err)
	}

	fmt.Println("Your new seed phrase is:")
	fmt.Println(mnemonic)
	fmt.Println("Please write this down and keep it safe.")

	fmt.Print("Enter a password to encrypt your wallet: ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("error reading password: %v", err)
	}
	password := strings.TrimSpace(string(passwordBytes))
	fmt.Println() // Add newline after password input

	encryptedMnemonic := utils.Encrypt(mnemonic, password)

	pubKey := ""
	apiKey := ""

	if !viper.GetBool("relay_wallet_set") {
		// Ask if the wallet will be used with the panel
		fmt.Print("Will this wallet be used with the panel? (yes/no): ")
		useWithPanel, _ := reader.ReadString('\n')
		useWithPanel = strings.TrimSpace(strings.ToLower(useWithPanel))

		if useWithPanel == "yes" {
			panelWallet := true

			fmt.Print("Enter your pubkey: ")
			pubKey, _ = reader.ReadString('\n')
			pubKey = strings.TrimSpace(pubKey)

			fmt.Print("Enter your API key: ")
			apiKey, _ = reader.ReadString('\n')
			apiKey = strings.TrimSpace(apiKey)

			// Set panel wallet configuration
			viper.Set("relay_wallet_set", panelWallet)
			viper.Set("wallet_name", walletName)
			viper.Set("wallet_api_key", apiKey)
			viper.Set("user_pubkey", pubKey)
		}
	}

	// Set the newly imported wallet flag to false for new wallets
	viper.Set("is_newly_imported", false)
	fmt.Println("Setting wallet as new (not imported) for standard sync timeouts")

	err = viper.WriteConfig()
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	// Generate public passphrase
	pubPass, err := generateRandomPassphrase(16)
	if err != nil {
		return fmt.Errorf("error generating public passphrase: %v", err)
	}

	// Generate private passphrase
	privPass, err := generateRandomPassphrase(32)
	if err != nil {
		return fmt.Errorf("error generating private passphrase: %v", err)
	}

	encryptedPubPass := utils.Encrypt(pubPass, password)
	encryptedPrivPass := utils.Encrypt(privPass, password)

	// Set birthdate to current date and time
	birthdate := time.Now().UTC()
	encryptedBirthdate := utils.Encrypt(birthdate.Format(timeFormat), password)

	// Save wallet data along with panel-specific info (if applicable)
	operations.SaveWalletData(walletName, encryptedMnemonic, encryptedPubPass, encryptedPrivPass, encryptedBirthdate)

	fmt.Printf("Wallet '%s' created and encrypted successfully.\n", walletName)
	fmt.Printf("Wallet birthdate: %s\n", birthdate.Format("2006-01-02"))

	return nil
}

func CreateWalletAPI(walletName, password, pubKey, apiKey string) (string, error) {
	log.Printf("Creating new wallet: %s", walletName)

	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}

	entropy, err := bip39.NewEntropy(256)
	if err != nil {
		return "", fmt.Errorf("error generating entropy: %v", err)
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("error generating mnemonic: %v", err)
	}

	encryptedMnemonic := utils.Encrypt(mnemonic, password)

	// Set panel wallet configuration if pubKey and apiKey are provided
	if pubKey != "" && apiKey != "" {
		viper.Set("relay_wallet_set", true)
		viper.Set("wallet_name", walletName)
		viper.Set("wallet_api_key", apiKey)
		viper.Set("user_pubkey", pubKey)
	}

	// Set the newly imported wallet flag to false for new wallets
	viper.Set("is_newly_imported", false)
	log.Printf("Setting is_newly_imported flag to false for new wallet: %s", walletName)

	err = viper.WriteConfig()
	if err != nil {
		return "", fmt.Errorf("error writing config file: %w", err)
	}

	// Generate public passphrase
	pubPass, err := generateRandomPassphrase(16)
	if err != nil {
		return "", fmt.Errorf("error generating public passphrase: %v", err)
	}

	// Generate private passphrase
	privPass, err := generateRandomPassphrase(32)
	if err != nil {
		return "", fmt.Errorf("error generating private passphrase: %v", err)
	}

	encryptedPubPass := utils.Encrypt(pubPass, password)
	encryptedPrivPass := utils.Encrypt(privPass, password)

	// Set birthdate to current date and time
	birthdate := time.Now().UTC()
	encryptedBirthdate := utils.Encrypt(birthdate.Format(timeFormat), password)

	// Save wallet data
	operations.SaveWalletData(walletName, encryptedMnemonic, encryptedPubPass, encryptedPrivPass, encryptedBirthdate)

	log.Printf("Wallet '%s' created and encrypted successfully.", walletName)
	log.Printf("Wallet birthdate: %s", birthdate.Format("2006-01-02"))

	return mnemonic, nil
}

func ExistingWallet(reader *bufio.Reader) error {

	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}

	fmt.Print("Enter a name for your existing wallet: ")
	walletName, _ := reader.ReadString('\n')
	walletName = strings.TrimSpace(walletName)

	fmt.Print("Enter your existing seed phrase: ")
	mnemonic, _ := reader.ReadString('\n')
	mnemonic = strings.TrimSpace(mnemonic)

	if !isValidMnemonic(mnemonic) {
		return fmt.Errorf("invalid mnemonic provided")
	}

	fmt.Print("Enter your wallet's birthdate (YYYY-MM-DD): ")
	birthdateStr, _ := reader.ReadString('\n')
	birthdateStr = strings.TrimSpace(birthdateStr)
	birthdate, err := time.Parse("2006-01-02", birthdateStr)
	if err != nil {
		return fmt.Errorf("invalid date format: %v", err)
	}

	fmt.Print("Enter a password to encrypt your wallet: ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("error reading password: %v", err)
	}
	password := strings.TrimSpace(string(passwordBytes))
	fmt.Println() // Add newline after password input

	encryptedMnemonic := utils.Encrypt(mnemonic, password)
	encryptedBirthdate := utils.Encrypt(birthdate.Format(timeFormat), password)

	pubKey := ""
	apiKey := ""

	if !viper.GetBool("relay_wallet_set") {
		// Ask if the wallet will be used with the panel
		fmt.Print("Will this wallet be used with the panel? (yes/no): ")
		useWithPanel, _ := reader.ReadString('\n')
		useWithPanel = strings.TrimSpace(strings.ToLower(useWithPanel))

		if useWithPanel == "yes" {
			panelWallet := true

			fmt.Print("Enter your pubkey: ")
			pubKey, _ = reader.ReadString('\n')
			pubKey = strings.TrimSpace(pubKey)

			fmt.Print("Enter your API key: ")
			apiKey, _ = reader.ReadString('\n')
			apiKey = strings.TrimSpace(apiKey)

			// Set panel wallet configuration
			viper.Set("relay_wallet_set", panelWallet)
			viper.Set("wallet_name", walletName)
			viper.Set("wallet_api_key", apiKey)
			viper.Set("user_pubkey", pubKey)
		}
	}

	// Set the newly imported wallet flag to true
	viper.Set("is_newly_imported", true)
	fmt.Println("Setting wallet as newly imported for extended initial sync timeouts")

	err = viper.WriteConfig()
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	// Generate public passphrase
	pubPass, err := generateRandomPassphrase(16)
	if err != nil {
		return fmt.Errorf("error generating public passphrase: %v", err)
	}

	// Generate private passphrase
	privPass, err := generateRandomPassphrase(32)
	if err != nil {
		return fmt.Errorf("error generating private passphrase: %v", err)
	}

	encryptedPubPass := utils.Encrypt(pubPass, password)
	encryptedPrivPass := utils.Encrypt(privPass, password)

	operations.SaveWalletData(walletName, encryptedMnemonic, encryptedPubPass, encryptedPrivPass, encryptedBirthdate)

	fmt.Printf("Existing wallet '%s' encrypted and saved successfully.\n", walletName)

	return nil
}

func ImportWalletAPI(walletName, mnemonic, password, birthdate, pubKey, apiKey string) error {
	// Validate the mnemonic
	if !isValidMnemonic(mnemonic) {
		return fmt.Errorf("invalid mnemonic provided")
	}

	// Parse the birthdate
	parsedBirthdate, err := time.Parse("2006-01-02", birthdate)
	if err != nil {
		return fmt.Errorf("invalid date format: %v", err)
	}

	encryptedMnemonic := utils.Encrypt(mnemonic, password)
	encryptedBirthdate := utils.Encrypt(parsedBirthdate.Format(timeFormat), password)

	if pubKey != "" && apiKey != "" {
		viper.Set("relay_wallet_set", false)
		viper.Set("wallet_name", walletName)
		viper.Set("wallet_api_key", apiKey)
		viper.Set("user_pubkey", pubKey)
	}

	// Set the newly imported wallet flag to true
	viper.Set("is_newly_imported", true)
	log.Printf("Setting is_newly_imported flag to true for wallet: %s", walletName)

	err = viper.WriteConfig()
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	// Generate public passphrase
	pubPass, err := generateRandomPassphrase(16)
	if err != nil {
		return fmt.Errorf("error generating public passphrase: %v", err)
	}

	// Generate private passphrase
	privPass, err := generateRandomPassphrase(32)
	if err != nil {
		return fmt.Errorf("error generating private passphrase: %v", err)
	}

	encryptedPubPass := utils.Encrypt(pubPass, password)
	encryptedPrivPass := utils.Encrypt(privPass, password)

	operations.SaveWalletData(walletName, encryptedMnemonic, encryptedPubPass, encryptedPrivPass, encryptedBirthdate)

	return nil
}

func isValidMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(mnemonic)
}

func generateRandomPassphrase(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
