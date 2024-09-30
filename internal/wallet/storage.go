package wallet

import (
	"log"
	"os"
	"path/filepath"

	"bufio"
	"fmt"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

const (
	walletDir  = "./wallets"
	timeFormat = "2006-01-02T15:04:05Z"
)

func saveWalletData(walletName, encryptedSeedPhrase, encryptedPubPass, encryptedPrivPass, encryptedBirthdate string) {
	err := os.MkdirAll(walletDir, os.ModePerm)
	if err != nil {
		log.Fatalf("Error creating wallet directory: %v", err)
	}

	envFile := filepath.Join(walletDir, walletName+".env")
	err = godotenv.Write(map[string]string{
		"ENCRYPTED_SEED_PHRASE":        encryptedSeedPhrase,
		"ENCRYPTED_PUBLIC_PASSPHRASE":  encryptedPubPass,
		"ENCRYPTED_PRIVATE_PASSPHRASE": encryptedPrivPass,
		"ENCRYPTED_BIRTHDATE":          encryptedBirthdate,
	}, envFile)

	if err != nil {
		log.Fatalf("Error saving encrypted data: %v", err)
	}
}

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

func LoadWalletAPI(walletName, password string) (string, string, string, time.Time, error) {
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

// deleteWallet handles the deletion of a specific wallet after password verification.
func DeleteWallet(reader *bufio.Reader) error {
	// Prompt for the wallet name
	fmt.Print("Enter the name of the wallet to delete: ")
	walletName, _ := reader.ReadString('\n')
	walletName = strings.TrimSpace(walletName)

	// Load the wallet's .env file
	walletDir := viper.GetString("wallet_dir") // Get the wallet directory path from the config
	envFile := filepath.Join(walletDir, walletName+".env")
	err := godotenv.Load(envFile)
	if err != nil {
		return fmt.Errorf("error loading wallet file: %v", err)
	}

	// Retrieve the encrypted seed phrase from the .env file
	encryptedSeedPhrase := os.Getenv("ENCRYPTED_SEED_PHRASE")
	if encryptedSeedPhrase == "" {
		return fmt.Errorf("encrypted seed phrase not found")
	}

	// Prompt for the wallet password
	fmt.Print("Enter your wallet password: ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	// Attempt to decrypt the seed phrase using the provided password
	_, err = decrypt(encryptedSeedPhrase, password)
	if err != nil {
		return fmt.Errorf("error decrypting seed phrase: incorrect password or decryption failed")
	}

	// Confirm the deletion
	fmt.Print("Are you sure you want to delete this wallet? This action cannot be undone. (y/n): ")
	confirmation, _ := reader.ReadString('\n')
	confirmation = strings.ToLower(strings.TrimSpace(confirmation))

	if confirmation != "y" {
		fmt.Println("Wallet deletion cancelled.")
		return nil
	}

	// Delete wallet files
	err = deleteWalletFiles(walletName)
	if err != nil {
		return fmt.Errorf("error deleting wallet files: %v", err)
	}

	fmt.Println("Wallet deleted successfully.")
	return nil
}

// deleteWalletFiles deletes all wallet-related files and directories for a given wallet name.
func deleteWalletFiles(walletName string) error {
	// Get the base directories from the configuration
	baseDir := viper.GetString("base_dir")                 // Base directory for general wallet-related files
	walletDir := viper.GetString("wallet_dir")             // Directory containing .env files
	neutrinoDbDir := filepath.Join(baseDir, "neutrino_db") // Neutrino database directory
	jwtKeysDir := filepath.Join(baseDir, "jwtkeys")        // JWT keys directory

	// Paths for wallet-specific files and directories
	envFile := filepath.Join(walletDir, walletName+".env")                                     // .env file
	neutrinoWalletDir := filepath.Join(neutrinoDbDir, fmt.Sprintf("%s_wallet", walletName))    // Neutrino wallet directory
	gravitonDbFile := filepath.Join(baseDir, fmt.Sprintf("%s_wallet_graviton.db", walletName)) // Graviton DB file
	jwtKeyDir := filepath.Join(jwtKeysDir, walletName)                                         // JWT key directory

	// Delete the wallet .env file
	if err := os.Remove(envFile); err != nil {
		log.Printf("Failed to delete .env file: %v", err) // Continue even if env file removal fails
	} else {
		log.Printf("Successfully deleted .env file: %s", envFile)
	}

	// Delete the Neutrino wallet directory and its contents
	if err := os.RemoveAll(neutrinoWalletDir); err != nil {
		log.Printf("Failed to delete Neutrino wallet directory: %v", err)
	} else {
		log.Printf("Successfully deleted Neutrino wallet directory: %s", neutrinoWalletDir)
	}

	// Delete the Graviton DB file
	if err := os.RemoveAll(gravitonDbFile); err != nil {
		log.Printf("Failed to delete Graviton DB file: %v", err)
	} else {
		log.Printf("Successfully deleted Graviton DB file or directory: %s", gravitonDbFile)
	}

	// Delete the JWT key directory and its contents
	if err := os.RemoveAll(jwtKeyDir); err != nil {
		log.Printf("Failed to delete JWT key directory: %v", err)
	} else {
		log.Printf("Successfully deleted JWT key directory: %s", jwtKeyDir)
	}

	return nil
}