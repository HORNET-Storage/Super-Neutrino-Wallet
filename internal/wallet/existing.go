package wallet

import (
	"bufio"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/spf13/viper"
)

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

	fmt.Print("Enter your wallet's birthdate (YYYY-MM-DD): ")
	birthdateStr, _ := reader.ReadString('\n')
	birthdateStr = strings.TrimSpace(birthdateStr)
	birthdate, err := time.Parse("2006-01-02", birthdateStr)
	if err != nil {
		return fmt.Errorf("invalid date format: %v", err)
	}

	fmt.Print("Enter a password to encrypt your wallet: ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	encryptedMnemonic := encrypt(mnemonic, password)
	encryptedBirthdate := encrypt(birthdate.Format(timeFormat), password)

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

	saveWalletData(walletName, encryptedMnemonic, encryptedPubPass, encryptedPrivPass, encryptedBirthdate)

	fmt.Printf("Existing wallet '%s' encrypted and saved successfully.\n", walletName)

	return nil
}

// package wallet

// import (
// 	"bufio"
// 	"fmt"
// 	"strings"
// 	"time"
// )

// func ExistingWallet(reader *bufio.Reader) error {
// 	fmt.Print("Enter a name for your existing wallet: ")
// 	walletName, _ := reader.ReadString('\n')
// 	walletName = strings.TrimSpace(walletName)

// 	fmt.Print("Enter your existing seed phrase: ")
// 	mnemonic, _ := reader.ReadString('\n')
// 	mnemonic = strings.TrimSpace(mnemonic)

// 	fmt.Print("Enter your wallet's birthdate (YYYY-MM-DD): ")
// 	birthdateStr, _ := reader.ReadString('\n')
// 	birthdateStr = strings.TrimSpace(birthdateStr)
// 	birthdate, err := time.Parse("2006-01-02", birthdateStr)
// 	if err != nil {
// 		return fmt.Errorf("invalid date format: %v", err)
// 	}

// 	fmt.Print("Enter a password to encrypt your wallet: ")
// 	password, _ := reader.ReadString('\n')
// 	password = strings.TrimSpace(password)

// 	encryptedMnemonic := encrypt(mnemonic, password)
// 	encryptedBirthdate := encrypt(birthdate.Format(timeFormat), password)

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

// 	saveWalletData(walletName, encryptedMnemonic, encryptedPubPass, encryptedPrivPass, encryptedBirthdate)

// 	fmt.Printf("Existing wallet '%s' encrypted and saved successfully.\n", walletName)

// 	return nil
// }
