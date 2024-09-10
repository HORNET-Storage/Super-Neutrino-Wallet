package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/wallet"
)

func PrintAddresses(addrType string, addresses []btcutil.Address) {
	log.Printf("%s addresses:", addrType)
	for i, addr := range addresses {
		log.Printf("%s address %d: %s", addrType, i, addr)
	}
}

func CleanupExistingData(neutrinoDBPath, walletDBPath string) error {
	if err := os.RemoveAll(neutrinoDBPath); err != nil {
		return fmt.Errorf("failed to remove Neutrino database directory: %v", err)
	}
	if err := os.MkdirAll(neutrinoDBPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to recreate Neutrino database directory: %v", err)
	}
	walletDir := filepath.Dir(walletDBPath)
	if err := os.MkdirAll(walletDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to recreate wallet directory: %v", err)
	}
	return nil
}

func GenerateAndSaveAddresses(w *wallet.Wallet, count int) ([]btcutil.Address, []btcutil.Address, error) {
	account := uint32(0)
	scope := waddrmgr.KeyScopeBIP0084

	// Load the most recent snapshot
	ss, err := walletstatedb.Store.LoadSnapshot(0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load snapshot: %v", err)
	}

	// Get the receive addresses tree
	receiveAddrTree, err := ss.GetTree("receive_addresses")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get receive addresses tree: %v", err)
	}

	// Get the change addresses tree
	changeAddrTree, err := ss.GetTree("change_addresses")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get change addresses tree: %v", err)
	}

	// Get the last receive address index
	lastReceiveIndex, err := walletstatedb.GetLastAddressIndex(receiveAddrTree)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting last receive address index: %v", err)
	}

	// Get the last change address index
	lastChangeIndex, err := walletstatedb.GetLastAddressIndex(changeAddrTree)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting last change address index: %v", err)
	}

	// Generate new addresses
	newReceiveAddresses := make([]btcutil.Address, count)
	newChangeAddresses := make([]btcutil.Address, count)

	for i := 0; i < count; i++ {
		// Generate a new receive address
		receiveAddr, err := w.NewAddress(account, scope)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate receive address: %v", err)
		}
		newReceiveAddresses[i] = receiveAddr

		// Save the receive address
		err = walletstatedb.SaveAddress(receiveAddrTree, walletstatedb.Address{
			Index:   uint(lastReceiveIndex + i + 1),
			Address: receiveAddr.String(),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to save receive address: %v", err)
		}

		// Generate a new change address
		changeAddr, err := w.NewChangeAddress(account, scope)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate change address: %v", err)
		}
		newChangeAddresses[i] = changeAddr

		// Save the change address
		err = walletstatedb.SaveAddress(changeAddrTree, walletstatedb.Address{
			Index:   uint(lastChangeIndex + i + 1),
			Address: changeAddr.String(),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to save change address: %v", err)
		}
	}

	// Commit the changes to both trees
	err = walletstatedb.CommitTrees(receiveAddrTree, changeAddrTree)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to commit trees: %v", err)
	}

	return newReceiveAddresses, newChangeAddresses, nil
}
