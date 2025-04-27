package main

import (
	"fmt"
	"os"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	_ "github.com/btcsuite/btcwallet/walletdb/bdb"
)

func main() {
	// Set SQLite as the default database backend
	walletstatedb.SetDatabaseBackend(walletstatedb.DBTypeSQLite)
	
	// Init config first so we can get paths
	initConfig()
	
	// Run SQLite initialization and migration if needed
	if err := InitializeSQLite(); err != nil {
		fmt.Printf("Warning: SQLite initialization error: %v\n", err)
	}
	if len(os.Args) > 1 {
		// CLI mode
		if err := rootCmd.Execute(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	} else {
		// Interactive mode
		interactiveMode()
	}
}
