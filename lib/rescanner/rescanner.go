package rescanner

import (
	"fmt"
	"log"
	"time"

	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/logger"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/utils"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/neutrino"
	"github.com/lightninglabs/neutrino/headerfs"
	"github.com/spf13/viper"
)

func PerformRescan(config RescanConfig) error {
	log.Println("Starting transaction recovery process")
	logger.Info("Starting transaction recovery process")

	// Check for nil values early
	if config.Wallet == nil || config.ChainClient == nil {
		return fmt.Errorf("wallet or ChainClient is nil, cannot proceed with rescan")
	}

	// Parse last sync time from the config
	lastSyncTimeStr := viper.GetString("last_sync_time")
	var syncTimeoutDuration time.Duration

	if lastSyncTimeStr == "" {
		// If last sync time is empty, set timeout to 3 minutes
		syncTimeoutDuration = 3 * time.Minute
	} else {
		lastSyncTime, err := time.Parse(time.RFC3339, lastSyncTimeStr)
		if err != nil {
			log.Printf("Error parsing last sync time: %v", err)
			syncTimeoutDuration = 1 * time.Minute
		} else {
			// If the last sync was more than 8 hours ago, set timeout to 1 minute
			if time.Since(lastSyncTime) > 8*time.Hour {
				syncTimeoutDuration = 1 * time.Minute
			} else {
				// Otherwise, set timeout to 30 seconds
				syncTimeoutDuration = 30 * time.Second
			}
		}
	}

	// Create a channel to capture any errors from the goroutine
	syncErrChan := make(chan error, 1)

	// Start the wallet synchronization process in a goroutine
	go func() {
		defer close(syncErrChan)
		config.Wallet.SynchronizeRPC(config.ChainClient)
	}()

	// Wait for initial sync to complete or timeout
	syncTimeout := time.After(5 * time.Second)
SyncLoop:
	for {
		select {
		case err := <-syncErrChan:
			if err != nil {
				log.Printf("Error in wallet synchronization: %v", err)
				logger.Error("Error in wallet synchronization: ", err)
				break SyncLoop
			}
		case <-syncTimeout:
			log.Println("Initial sync timeout reached, proceeding with address scanning")
			break SyncLoop
		default:
			if !config.Wallet.SynchronizingToNetwork() {
				log.Println("Initial sync completed, proceeding with address scanning")
				break SyncLoop
			}
			time.Sleep(time.Second)
		}
	}

	// Retrieve all addresses from the wallet
	allAddresses, err := config.Wallet.AccountAddresses(0)
	if err != nil {
		return fmt.Errorf("failed to get addresses from wallet: %v", err)
	}

	log.Printf("Rescanning with %d addresses", len(allAddresses))
	logger.Info("Rescanning addresses: ", len(allAddresses))

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
	logger.Info("Starting address scanning...")

	knownTxs := make(map[chainhash.Hash]*btcutil.Tx)
	for _, addr := range allAddresses {
		err := rescanAddress(chainSource, addr.String(), config.StartBlock, bestHeight, quit, knownTxs)
		if err != nil {
			log.Printf("Error scanning address %s: %v", addr.String(), err)
			continue
		}
		logger.Info(fmt.Sprintf("Rescan completed for address %s", addr))
	}

	logger.Info("Address scanning complete...")

	// Wait for full synchronization to complete or timeout
	fullSyncTimeout := time.After(syncTimeoutDuration)
FullSyncLoop:
	for {
		select {
		case <-fullSyncTimeout:
			log.Println("Wallet synchronization timed out, but address scanning completed")
			logger.Info("Wallet synchronization timed out, but address scanning completed")
			break FullSyncLoop
		default:
			if !config.Wallet.SynchronizingToNetwork() {
				log.Println("Wallet synchronization completed successfully")
				logger.Info("Wallet synchronization completed successfully")
				break FullSyncLoop
			}
			time.Sleep(time.Second)
		}
	}

	// After rescan and synchronization, update the last sync time
	err = utils.SetLastSyncTime(time.Now())
	if err != nil {
		log.Printf("Error updating last sync time: %v", err)
	}

	// After synchronization, get the updated balance from our database
	balance, err := config.Wallet.CalculateBalance(1)
	if err != nil {
		return fmt.Errorf("failed to get wallet balance: %v", err)
	}

	log.Printf("Final wallet balance after rescan: %d satoshis", balance)
	log.Println("Transaction recovery process completed")
	logger.Info("Transaction recovery process completed")

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
