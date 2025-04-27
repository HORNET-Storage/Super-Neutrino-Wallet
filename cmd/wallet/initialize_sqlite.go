package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/spf13/viper"
)

// InitializeSQLite checks if we need to migrate from Graviton to SQLite
func InitializeSQLite() error {
	// Get wallet data directory from config
	baseDir := getWalletBaseDir()
	if baseDir == "" {
		log.Println("Wallet directory not configured, using default")
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %v", err)
		}
		baseDir = filepath.Join(homeDir, ".sn-wallet")
	}

	// Get wallet name from config
	walletName := getWalletName()
	if walletName == "" {
		log.Println("Wallet name not configured, using default")
		walletName = "default"
	}

	// Check if Graviton DB exists and SQLite DB doesn't exist
	gravitonDBPath := filepath.Join(baseDir, fmt.Sprintf("%s_wallet_graviton.db", walletName))
	sqliteDBPath := filepath.Join(baseDir, fmt.Sprintf("%s_wallet.db", walletName))

	gravitonExists := fileExists(gravitonDBPath)
	sqliteExists := fileExists(sqliteDBPath)

	if gravitonExists && !sqliteExists {
		// We need to migrate from Graviton to SQLite
		log.Println("Migrating data from Graviton to SQLite...")
		
		// Temporarily switch backend to allow migration
		originalBackend := walletstatedb.DBBackend
		defer func() {
			walletstatedb.SetDatabaseBackend(originalBackend)
		}()
		
		// Run migration
		if err := walletstatedb.RunMigration(baseDir, walletName); err != nil {
			return fmt.Errorf("migration failed: %v", err)
		}
		
		log.Println("Migration completed successfully")
	} else if !sqliteExists {
		// No existing database, SQLite will be created when needed
		log.Println("No existing database found, new SQLite database will be created when needed")
	} else {
		// SQLite database already exists
		log.Println("SQLite database already exists")
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

// getWalletBaseDir retrieves the wallet directory from configuration
func getWalletBaseDir() string {
	// Get from viper config - adjust path as needed for your config
	return viper.GetString("wallet_dir")
}

// getWalletName retrieves the wallet name from configuration
func getWalletName() string {
	// Get from viper config - adjust path as needed for your config
	return viper.GetString("wallet_name")
}