package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// InitializeSQLite initializes the SQLite database
func InitializeSQLite() error {
	// Get wallet data directory from config
	baseDir := getWalletBaseDir()
	log.Printf("Got base directory from config: %q", baseDir)
	if baseDir == "" {
		log.Println("Wallet directory not configured, using default")
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %v", err)
		}
		baseDir = filepath.Join(homeDir, ".sn-wallet")
	}
	log.Printf("Using base directory: %s", baseDir)

	// Get wallet name from config
	walletName := getWalletName()
	log.Printf("Got wallet name from config: %q", walletName)
	if walletName == "" {
		log.Println("Wallet name not configured, using default")
		walletName = "default"
	}

	// Construct the SQLite DB path
	sqliteDBPath := filepath.Join(baseDir, fmt.Sprintf("%s_wallet.db", walletName))
	log.Printf("SQLite DB path: %s", sqliteDBPath)

	// Check if SQLite DB exists
	sqliteExists := fileExists(sqliteDBPath)
	
	// Special case for legacy database path
	if !sqliteExists {
		legacyPath := "/Users/siphiwetapisi/my_go/wallet-cleanup/btc-wallet-btcsuite/zambi_wallet.db"
		if fileExists(legacyPath) {
			sqliteDBPath = legacyPath
			sqliteExists = true
			walletName = "zambi"
			log.Printf("Found SQLite DB at legacy path: %s", sqliteDBPath)
		}
	}

	if !sqliteExists {
		// No existing database, SQLite will be created when needed
		log.Println("No existing database found, new SQLite database will be created when needed")
	} else {
		// SQLite database already exists
		log.Println("SQLite database exists")
	}

	return nil
}

// Helper function to check if a file exists
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// We don't need the initDB function anymore as SQLite is initialized 
// elsewhere in the codebase when needed

// getWalletBaseDir retrieves the wallet directory from configuration
func getWalletBaseDir() string {
	// First try the wallet_dir config
	dir := viper.GetString("wallet_dir")
	if dir != "" {
		return dir
	}
	
	// Then try base_dir which is always set in initConfig
	return viper.GetString("base_dir")
}

// getWalletName retrieves the wallet name from configuration
func getWalletName() string {
	// When we're in interactive mode, we need to check where we are in the process
	walletName := os.Getenv("WALLET_NAME")
	if walletName != "" {
		return walletName
	}
	
	// Otherwise, check viper
	return viper.GetString("wallet_name")
}