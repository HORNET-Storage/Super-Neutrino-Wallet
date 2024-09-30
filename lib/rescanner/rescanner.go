package rescanner

import (
	"fmt"
	"log"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/neutrino"
	"github.com/lightninglabs/neutrino/headerfs"
)

func PerformRescan(config RescanConfig) error {
	log.Println("Starting transaction recovery process")

	// Start the wallet synchronization process
	go config.Wallet.SynchronizeRPC(config.ChainClient)

	// Wait for initial sync to complete or timeout
	syncTimeout := time.After(time.Second * 5) // Adjust timeout as needed
	for {
		select {
		case <-syncTimeout:
			log.Println("Initial sync timeout reached, proceeding with address scanning")
			goto AddressScanning
		default:
			if !config.Wallet.SynchronizingToNetwork() {
				log.Println("Initial sync completed, proceeding with address scanning")
				goto AddressScanning
			}
			time.Sleep(time.Second)
		}
	}

AddressScanning:
	// Retrieve all addresses from the wallet
	allAddresses, err := config.Wallet.AccountAddresses(0)
	if err != nil {
		return fmt.Errorf("failed to get addresses from wallet: %v", err)
	}

	log.Printf("Rescanning with %d addresses", len(allAddresses))

	// Create a map of addresses for faster lookup
	walletAddressMap := make(map[string]bool)
	for _, addr := range allAddresses {
		walletAddressMap[addr.EncodeAddress()] = true
	}

	// Get the current best block height
	_, bestHeight, err := config.ChainClient.GetBestBlock()
	if err != nil {
		return fmt.Errorf("failed to get best block: %v", err)
	}

	// Create a RescanChainSource from the ChainClient
	chainSource := &neutrino.RescanChainSource{ChainService: config.ChainClient.CS}

	quit := make(chan struct{})
	defer close(quit)

	// Rescan addresses
	log.Println("Starting address scanning...")

	knownTxs := make(map[chainhash.Hash]*btcutil.Tx)
	for _, addr := range allAddresses {
		err := rescanAddress(chainSource, addr.String(), config.StartBlock, bestHeight, quit, knownTxs)
		if err != nil {
			log.Printf("Error scanning address %s: %v", addr.String(), err)
			continue
		}
	}

	// Wait for full synchronization to complete or timeout
	fullSyncTimeout := time.After(time.Minute * 1) // Adjust final timeout as needed
	for {
		select {
		case <-fullSyncTimeout:
			log.Println("Wallet synchronization timed out, but address scanning completed")
			goto Finish
		default:
			if !config.Wallet.SynchronizingToNetwork() {
				log.Println("Wallet synchronization completed successfully")
				goto Finish
			}
			time.Sleep(time.Second)
		}
	}

Finish:
	// After synchronization, get the updated balance from our database
	balance, err := config.Wallet.CalculateBalance(1)
	if err != nil {
		return fmt.Errorf("failed to get wallet balance: %v", err)
	}

	log.Printf("Final wallet balance after rescan: %d satoshis", balance)

	log.Println("Transaction recovery process completed")
	return nil
}

func rescanAddress(cs *neutrino.RescanChainSource, address string, startHeight, endHeight int32, quit chan struct{}, knownTxs map[chainhash.Hash]*btcutil.Tx) error {
	addr, err := btcutil.DecodeAddress(address, &chaincfg.MainNetParams)
	if err != nil {
		return err
	}

	ntfn := rpcclient.NotificationHandlers{
		OnFilteredBlockConnected: func(height int32, header *wire.BlockHeader, txns []*btcutil.Tx) {
			for _, tx := range txns {
				knownTxs[*tx.Hash()] = tx
				amountReceived, amountSent := calculateTransactionAmounts(tx, address, knownTxs)

				if amountReceived > 0 || amountSent > 0 {
					log.Printf("Transaction found in block %d: %s", height, tx.Hash())
					log.Printf("Received amount: %s, Sent amount: %s", amountReceived.String(), amountSent.String())
					log.Println("Transaction date: ", header.Timestamp.Format("2006-01-02 15:04:05"))
				}
			}
		},
	}

	rescan := neutrino.NewRescan(
		cs,
		neutrino.StartBlock(&headerfs.BlockStamp{Height: startHeight}),
		neutrino.EndBlock(&headerfs.BlockStamp{Height: endHeight}),
		neutrino.WatchAddrs(addr),
		neutrino.NotificationHandlers(ntfn),
		neutrino.QuitChan(quit),
		neutrino.QueryOptions(
			neutrino.NumRetries(10),
			neutrino.Timeout(time.Minute*20),
		),
	)
	errChan := rescan.Start()

	select {
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("rescan error: %v", err)
		}
		log.Printf("Rescan completed for address %s", address)
	case <-time.After(time.Minute * 30): // Adjust timeout as needed
		return fmt.Errorf("rescan timed out for address %s", address)
	}
	return nil
}

func calculateTransactionAmounts(tx *btcutil.Tx, address string, knownTxs map[chainhash.Hash]*btcutil.Tx) (btcutil.Amount, btcutil.Amount) {
	amountReceived := btcutil.Amount(0)
	amountSent := btcutil.Amount(0)

	for _, txOut := range tx.MsgTx().TxOut {
		_, addrs, _, err := txscript.ExtractPkScriptAddrs(txOut.PkScript, &chaincfg.MainNetParams)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if a.EncodeAddress() == address {
				amountReceived += btcutil.Amount(txOut.Value)
			}
		}
	}

	for _, txIn := range tx.MsgTx().TxIn {
		prevTx, ok := knownTxs[txIn.PreviousOutPoint.Hash]
		if !ok {
			continue
		}
		prevTxOut := prevTx.MsgTx().TxOut[txIn.PreviousOutPoint.Index]
		_, addrs, _, err := txscript.ExtractPkScriptAddrs(prevTxOut.PkScript, &chaincfg.MainNetParams)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if a.EncodeAddress() == address {
				amountSent += btcutil.Amount(prevTxOut.Value)
			}
		}
	}

	return amountReceived, amountSent
}
