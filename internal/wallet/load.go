package wallet

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	walletDir  = "./wallets"
	timeFormat = "2006-01-02T15:04:05Z"
)

func loadWallet(walletName string) (string, string, string, time.Time, error) {
	envFile := filepath.Join(walletDir, walletName+".env")
	err := godotenv.Load(envFile)
	if err != nil {
		return "", "", "", time.Time{}, fmt.Errorf("error loading wallet file: %v", err)
	}

	encryptedSeedPhrase := os.Getenv("ENCRYPTED_SEED_PHRASE")
	encryptedPubPass := os.Getenv("ENCRYPTED_PUBLIC_PASSPHRASE")
	encryptedPrivPass := os.Getenv("ENCRYPTED_PRIVATE_PASSPHRASE")
	encryptedBirthdate := os.Getenv("ENCRYPTED_BIRTHDATE")

	if encryptedSeedPhrase == "" || encryptedPubPass == "" || encryptedPrivPass == "" || encryptedBirthdate == "" {
		return "", "", "", time.Time{}, fmt.Errorf("encrypted wallet data not found")
	}

	fmt.Print("Enter your wallet password: ")
	reader := bufio.NewReader(os.Stdin)
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	seedPhrase, err := decrypt(encryptedSeedPhrase, password)
	if err != nil {
		return "", "", "", time.Time{}, fmt.Errorf("error decrypting seed phrase: %v", err)
	}

	pubPass, err := decrypt(encryptedPubPass, password)
	if err != nil {
		return "", "", "", time.Time{}, fmt.Errorf("error decrypting public passphrase: %v", err)
	}

	privPass, err := decrypt(encryptedPrivPass, password)
	if err != nil {
		return "", "", "", time.Time{}, fmt.Errorf("error decrypting private passphrase: %v", err)
	}

	birthdateStr, err := decrypt(encryptedBirthdate, password)
	if err != nil {
		return "", "", "", time.Time{}, fmt.Errorf("error decrypting birthdate: %v", err)
	}

	birthdate, err := time.Parse(timeFormat, birthdateStr)
	if err != nil {
		return "", "", "", time.Time{}, fmt.Errorf("error parsing birthdate: %v", err)
	}

	return seedPhrase, pubPass, privPass, birthdate, nil
}

func listWallets() ([]string, error) {
	files, err := os.ReadDir(walletDir)
	if err != nil {
		return nil, fmt.Errorf("error reading wallet directory: %v", err)
	}

	var wallets []string
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".env" {
			wallets = append(wallets, strings.TrimSuffix(file.Name(), ".env"))
		}
	}

	return wallets, nil
}
