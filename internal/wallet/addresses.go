package wallet

import (
	"fmt"
	"log"
	"time"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/lib/utils"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
)

func handleAddressGeneration(w *wallet.Wallet, chainClient *chain.NeutrinoClient, needsAddresses, freshWallet bool) error {
	var numberofAddr int
	if needsAddresses {
		numberofAddr = 30
		err := generateInitialAddresses(w, chainClient, numberofAddr)
		if err != nil {
			return fmt.Errorf("error generating initial addresses: %s", err)
		}
	} else if freshWallet {
		numberofAddr = 1
		err := generateInitialAddresses(w, chainClient, numberofAddr)
		if err != nil {
			return fmt.Errorf("error generating initial addresses: %s", err)
		}
	} else {
		log.Println("Using existing addresses from the database")
	}

	return nil
}

func generateInitialAddresses(w *wallet.Wallet, chainClient *chain.NeutrinoClient, numAddresses int) error {
	const maxRetries = 2
	var receiveAddresses, changeAddresses []btcutil.Address
	var err error

	for i := 0; i < maxRetries; i++ {
		log.Printf("Attempt %d: Generating initial addresses", i+1)
		receiveAddresses, changeAddresses, err = utils.GenerateAndSaveAddresses(w, numAddresses)
		if err == nil {
			log.Printf("Successfully generated and saved %d receive addresses and %d change addresses", len(receiveAddresses), len(changeAddresses))
			break
		}
		log.Printf("Error generating addresses: %v", err)
		if i < maxRetries-1 {
			log.Println("Retrying in 5 seconds...")
			time.Sleep(5 * time.Second)
		}
	}

	if err != nil {
		log.Printf("Failed to generate addresses after %d attempts", maxRetries)
		return fmt.Errorf("failed to generate addresses after %d attempts, with error: %s", maxRetries, err)
	} else {
		utils.PrintAddresses("Receive", receiveAddresses)
		utils.PrintAddresses("Change", changeAddresses)
	}

	_, chainClientBestblock, err := chainClient.GetBestBlock()
	if err != nil {
		return fmt.Errorf("error getting chain client best block: %v", err)
	} else {
		log.Printf("Chain client best block: %d", chainClientBestblock)
		err = walletstatedb.SetLastScannedBlockHeight(chainClientBestblock)
		if err != nil {
			return fmt.Errorf("error setting initial last scanned block height: %v", err)
		} else {
			log.Printf("Initial last scanned block height set to %d", chainClientBestblock)
		}
	}

	return nil
}
