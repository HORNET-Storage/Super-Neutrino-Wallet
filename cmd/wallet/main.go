package main

import (
	"fmt"
	"os"

	_ "github.com/btcsuite/btcwallet/walletdb/bdb"
)

func main() {
	initConfig()
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
