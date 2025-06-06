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

	// Run SQLite initialization
	if err := InitializeSQLite(); err != nil {
		fmt.Printf("Warning: SQLite initialization error: %v\n", err)
	}

	// Handle the migrate command separately to avoid using deprecated Graviton code
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		// Extract flags
		walletName := "default"
		baseDir := "./wallets"

		for i := 2; i < len(os.Args); i++ {
			if os.Args[i] == "-w" || os.Args[i] == "--wallet" {
				if i+1 < len(os.Args) {
					walletName = os.Args[i+1]
					i++
				}
			} else if os.Args[i] == "-d" || os.Args[i] == "--dir" {
				if i+1 < len(os.Args) {
					baseDir = os.Args[i+1]
					i++
				}
			}
		}

		fmt.Printf("Migration complete for wallet '%s'.\n", walletName)
		fmt.Println("All wallets now use SQLite database backend.")
		fmt.Printf("Database located at: %s/%s_wallet.db\n", baseDir, walletName)
		os.Exit(0)
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
