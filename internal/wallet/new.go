package wallet

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/tyler-smith/go-bip39"
)

func generateRandomPassphrase(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func CreateNewWallet(reader *bufio.Reader) error {
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

	// Set birthdate to current date and time
	birthdate := time.Now().UTC()
	encryptedBirthdate := encrypt(birthdate.Format(timeFormat), password)

	saveWalletData(walletName, encryptedMnemonic, encryptedPubPass, encryptedPrivPass, encryptedBirthdate)

	fmt.Printf("Wallet '%s' created and encrypted successfully.\n", walletName)
	fmt.Printf("Wallet birthdate: %s\n", birthdate.Format("2006-01-02"))

	return nil
}
