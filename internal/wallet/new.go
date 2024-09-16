package wallet

import (
	"bufio"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/tyler-smith/go-bip39"
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
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	encryptedMnemonic := encrypt(mnemonic, password)

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

	encryptedPubPass := encrypt(pubPass, password)
	encryptedPrivPass := encrypt(privPass, password)

	// Set birthdate to current date and time
	birthdate := time.Now().UTC()
	encryptedBirthdate := encrypt(birthdate.Format(timeFormat), password)

	// Save wallet data along with panel-specific info (if applicable)
	saveWalletData(walletName, encryptedMnemonic, encryptedPubPass, encryptedPrivPass, encryptedBirthdate)

	fmt.Printf("Wallet '%s' created and encrypted successfully.\n", walletName)
	fmt.Printf("Wallet birthdate: %s\n", birthdate.Format("2006-01-02"))

	return nil
}

// package wallet

// import (
// 	"bufio"
// 	"crypto/rand"
// 	"encoding/hex"
// 	"fmt"
// 	"strings"
// 	"time"

// 	"github.com/tyler-smith/go-bip39"
// )

// func generateRandomPassphrase(length int) (string, error) {
// 	bytes := make([]byte, length)
// 	if _, err := rand.Read(bytes); err != nil {
// 		return "", err
// 	}
// 	return hex.EncodeToString(bytes), nil
// }

// func CreateNewWallet(reader *bufio.Reader) error {
// 	fmt.Print("Enter a name for your new wallet: ")
// 	walletName, _ := reader.ReadString('\n')
// 	walletName = strings.TrimSpace(walletName)

// 	entropy, err := bip39.NewEntropy(256)
// 	if err != nil {
// 		return fmt.Errorf("error generating entropy: %v", err)
// 	}

// 	mnemonic, err := bip39.NewMnemonic(entropy)
// 	if err != nil {
// 		return fmt.Errorf("error generating mnemonic: %v", err)
// 	}

// 	fmt.Println("Your new seed phrase is:")
// 	fmt.Println(mnemonic)
// 	fmt.Println("Please write this down and keep it safe.")

// 	fmt.Print("Enter a password to encrypt your wallet: ")
// 	password, _ := reader.ReadString('\n')
// 	password = strings.TrimSpace(password)

// 	encryptedMnemonic := encrypt(mnemonic, password)

// 	// Generate public passphrase
// 	pubPass, err := generateRandomPassphrase(16) // 32 characters long
// 	if err != nil {
// 		return fmt.Errorf("error generating public passphrase: %v", err)
// 	}

// 	// Generate private passphrase
// 	privPass, err := generateRandomPassphrase(32) // 64 characters long
// 	if err != nil {
// 		return fmt.Errorf("error generating private passphrase: %v", err)
// 	}

// 	encryptedPubPass := encrypt(pubPass, password)
// 	encryptedPrivPass := encrypt(privPass, password)

// 	// Set birthdate to current date and time
// 	birthdate := time.Now().UTC()
// 	encryptedBirthdate := encrypt(birthdate.Format(timeFormat), password)

// 	saveWalletData(walletName, encryptedMnemonic, encryptedPubPass, encryptedPrivPass, encryptedBirthdate)

// 	fmt.Printf("Wallet '%s' created and encrypted successfully.\n", walletName)
// 	fmt.Printf("Wallet birthdate: %s\n", birthdate.Format("2006-01-02"))

// 	return nil
// }
