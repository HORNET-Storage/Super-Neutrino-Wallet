package wallet

import (
	"bufio"
	"fmt"
	"strings"
	"time"
)

func ExistingWallet(reader *bufio.Reader) error {
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

	// Generate public passphrase
	pubPass, err := generateRandomPassphrase(16) // 32 characters long
	if err != nil {
		return fmt.Errorf("error generating public passphrase: %v", err)
	}

	// Generate private passphrase
	privPass, err := generateRandomPassphrase(32) // 64 characters long
	if err != nil {
		return fmt.Errorf("error generating private passphrase: %v", err)
	}

	encryptedPubPass := encrypt(pubPass, password)
	encryptedPrivPass := encrypt(privPass, password)

	saveWalletData(walletName, encryptedMnemonic, encryptedPubPass, encryptedPrivPass, encryptedBirthdate)

	fmt.Printf("Existing wallet '%s' encrypted and saved successfully.\n", walletName)

	return nil
}
