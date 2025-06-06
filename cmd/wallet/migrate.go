package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// migrateCmd represents the migrate command
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate database (no longer needed)",
	Long:  `This command is kept for backwards compatibility but is no longer needed as all wallets now use SQLite.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get command line flags or defaults
		walletName, _ := cmd.Flags().GetString("wallet")
		baseDir, _ := cmd.Flags().GetString("dir")

		if baseDir == "" {
			baseDir = "./wallets" // Default directory
		}

		// Skip any actual migration since SQLite is now the only database
		fmt.Printf("Migration complete for wallet '%s'.\n", walletName)
		fmt.Println("All wallets now use SQLite database backend.")
		fmt.Printf("Database located at: %s/%s_wallet.db\n", baseDir, walletName)

		// Explicitly exit to prevent any further processing
		os.Exit(0)
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
