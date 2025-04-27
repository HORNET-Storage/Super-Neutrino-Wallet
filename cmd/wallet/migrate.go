package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// migrateCmd represents the migrate command
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate database from Graviton to SQLite",
	Long:  `Migrates wallet data from Graviton DB to SQLite database.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get command line flags or defaults
		walletName, _ := cmd.Flags().GetString("wallet")
		baseDir, _ := cmd.Flags().GetString("dir")

		// If wallet name not provided, use from config
		if walletName == "" {
			walletName = viper.GetString("wallet_name")
			if walletName == "" {
				log.Fatal("Wallet name not provided and not found in config")
			}
		}

		// If base directory not provided, use from config or default
		if baseDir == "" {
			baseDir = viper.GetString("wallet_dir")
			if baseDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					log.Fatalf("Failed to get user home directory: %v", err)
				}
				baseDir = filepath.Join(homeDir, ".sn-wallet")
			}
		}

		fmt.Printf("Migrating wallet '%s' data from Graviton to SQLite...\n", walletName)
		fmt.Printf("Base directory: %s\n", baseDir)

		// Run the migration
		err := walletstatedb.RunMigration(baseDir, walletName)
		if err != nil {
			log.Fatalf("Migration failed: %v", err)
		}

		fmt.Println("Migration completed successfully!")
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)

	// Add flags specific to this command
	migrateCmd.Flags().StringP("wallet", "w", "", "Wallet name to migrate")
	migrateCmd.Flags().StringP("dir", "d", "", "Base directory containing the wallet data")

	// Optional: validate
	migrateCmd.Flags().BoolP("validate", "v", false, "Validate data after migration")
}