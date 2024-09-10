package wallet

import (
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
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
