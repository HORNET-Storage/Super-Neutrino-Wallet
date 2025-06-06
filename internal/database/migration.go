package walletstatedb

import (
	"fmt"
	"log"
	"runtime"
)

// MigrateFromGravitonToSQLite is a stub function kept for backward compatibility
func MigrateFromGravitonToSQLite(gravitonDBPath, sqliteDBPath string) error {
	// Print the stack trace to help identify where this function is called from
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	fmt.Printf("TRACE: MigrateFromGravitonToSQLite called from:\n%s\n", buf[:n])

	log.Printf("MigrateFromGravitonToSQLite is deprecated. All wallets use SQLite now.")
	return nil
}

// RunMigration is a stub that indicates migration is no longer needed
func RunMigration(baseDir, walletName string) error {
	// Print the stack trace to help identify where this function is called from
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	fmt.Printf("TRACE: RunMigration called from:\n%s\n", buf[:n])

	log.Printf("Migration process bypassed. All wallets now use SQLite.")
	log.Printf("Database file: %s/%s_wallet.db", baseDir, walletName)
	return nil
}
