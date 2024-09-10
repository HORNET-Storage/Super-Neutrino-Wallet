package wallet

import (
	"bufio"
	"errors"
	"fmt"
)

func OpenAndloadWallet(reader *bufio.Reader, baseDir string) error {
	wallets, err := listWallets()
	if err != nil {
		return fmt.Errorf("error listing wallets: %v", err)
	}

	if len(wallets) == 0 {
		fmt.Println("No wallets found. Please create a new wallet first.")
		return errors.New("no wallets found. Please create a new wallet first")
	}

	fmt.Println("Available wallets:")
	for i, wallet := range wallets {
		fmt.Printf("%d. %s\n", i+1, wallet)
	}

	var choice int
	for {
		fmt.Print("Enter the number of the wallet you want to login to: ")
		_, err := fmt.Fscanf(reader, "%d\n", &choice)
		if err == nil && choice > 0 && choice <= len(wallets) {
			break
		} else {
			return errors.New("invalid choice. Please try again")
		}

	}

	walletName := wallets[choice-1]

	seedPhrase, publicPass, privatePass, birthdate, err := loadWallet(walletName)
	if err != nil {
		return fmt.Errorf("error loading wallet: %v", err)
	}

	fmt.Printf("Wallet '%s' loaded successfully.\n", walletName)

	pubPass := []byte(publicPass)
	privPass := []byte(privatePass)

	serverMode := true

	err = StartWallet(seedPhrase, pubPass, privPass, baseDir, walletName, birthdate, serverMode)
	if err != nil {
		return fmt.Errorf("failed to start wallet: %v", err)
	}
	return nil
}
