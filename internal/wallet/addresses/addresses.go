package addresses

import (
	"fmt"
	"log"
	"time"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/wallet"
)

// Constants
const (
	UnsentAddressesTree = "unsent_addresses"
)

func HandleAddressGeneration(w *wallet.Wallet, chainClient *chain.NeutrinoClient, needsAddresses, freshWallet bool) error {
	var numberofAddr int
	if needsAddresses {
		numberofAddr = 20
		err := GenerateInitialAddresses(w, chainClient, numberofAddr)
		if err != nil {
			return fmt.Errorf("error generating initial addresses: %s", err)
		}
	} else if freshWallet {
		numberofAddr = 10
		err := GenerateInitialAddresses(w, chainClient, numberofAddr)
		if err != nil {
			return fmt.Errorf("error generating initial addresses: %s", err)
		}
	} else {
		log.Println("Using existing addresses from the database")
	}

	return nil
}

func GenerateInitialAddresses(w *wallet.Wallet, chainClient *chain.NeutrinoClient, numAddresses int) error {
	const maxRetries = 2
	var receiveAddresses, changeAddresses []btcutil.Address
	var err error

	for i := 0; i < maxRetries; i++ {
		log.Printf("Attempt %d: Generating initial addresses", i+1)
		receiveAddresses, changeAddresses, err = GenerateAndSaveAddresses(w, numAddresses)
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
		PrintAddresses("Receive", receiveAddresses)
		PrintAddresses("Change", changeAddresses)
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

func PrintAddresses(addrType string, addresses []btcutil.Address) {
	log.Printf("%s addresses:", addrType)
	for i, addr := range addresses {
		log.Printf("%s address %d: %s", addrType, i, addr)
	}
}

func GenerateAndSaveAddresses(w *wallet.Wallet, count int) ([]btcutil.Address, []btcutil.Address, error) {
	account := uint32(0)
	scope := waddrmgr.KeyScopeBIP0084

	// Get the last address index for receive and change addresses
	lastReceiveIndex, err := walletstatedb.GetLastAddressIndexWithType("receive")
	if err != nil {
		return nil, nil, fmt.Errorf("error getting last receive address index: %v", err)
	}

	lastChangeIndex, err := walletstatedb.GetLastAddressIndexWithType("change")
	if err != nil {
		return nil, nil, fmt.Errorf("error getting last change address index: %v", err)
	}

	newReceiveAddresses := make([]btcutil.Address, count)
	newChangeAddresses := make([]btcutil.Address, count)

	for i := 0; i < count; i++ {
		// Generate and save receive address
		receiveAddr, err := w.NewAddress(account, scope)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate receive address: %v", err)
		}
		newReceiveAddresses[i] = receiveAddr

		// Create address struct
		addrStruct := walletstatedb.Address{
			Index:   uint(lastReceiveIndex + i + 1),
			Address: receiveAddr.String(),
			Status:  walletstatedb.AddressStatusAvailable,
		}

		// Save address
		err = walletstatedb.SaveAddressWithType("receive", addrStruct)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to save receive address: %v", err)
		}

		// Generate and save change address
		changeAddr, err := w.NewChangeAddress(account, scope)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate change address: %v", err)
		}
		newChangeAddresses[i] = changeAddr

		err = walletstatedb.SaveAddressWithType("change", walletstatedb.Address{
			Index:   uint(lastChangeIndex + i + 1),
			Address: changeAddr.String(),
			Status:  walletstatedb.AddressStatusAvailable,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to save change address: %v", err)
		}
	}

	return newReceiveAddresses, newChangeAddresses, nil
}

// GetUnsentAddresses retrieves all unsent addresses
func GetUnsentAddresses() ([]walletstatedb.Address, error) {
	// This function would be modified to use a flag in the SQLiteAddress model
	// or a separate table to track unsent addresses
	// For now, we'll get all addresses marked with 'available' status
	return walletstatedb.GetAddressesWithType("receive")
}

// ClearUnsentAddresses clears the unsent status of addresses
func ClearUnsentAddresses() error {
	// This would be implemented to mark addresses as sent in the database
	// For now, we'll return nil as a placeholder
	log.Println("ClearUnsentAddresses: this function needs implementation with SQLite")
	return nil
}
