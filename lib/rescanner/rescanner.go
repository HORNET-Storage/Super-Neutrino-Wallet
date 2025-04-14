package rescanner

import (
	"fmt"
	"log"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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
	log.Println("Starting optimized transaction recovery process")
	logger.Info("Starting optimized transaction recovery process")

	// Check for nil values early
	if config.Wallet == nil || config.ChainClient == nil {
		return fmt.Errorf("wallet or ChainClient is nil, cannot proceed with rescan")
	}

	// Determine the final wallet synchronization timeout
	var syncTimeoutDuration time.Duration

	// For newly imported wallets, use a longer timeout
	if config.IsImportedWallet {
		syncTimeoutDuration = 10 * time.Minute // 10 minutes for newly imported wallets
		log.Println("Using extended final sync timeout for newly imported wallet: 10 minutes")
	} else {
		// For regular wallets, use adaptive timeout based on last sync time
		lastSyncTimeStr := viper.GetString("last_sync_time")
		if lastSyncTimeStr == "" {
			// If last sync time is empty, set timeout to 3 minutes
			syncTimeoutDuration = 3 * time.Minute
		} else {
			lastSyncTime, err := time.Parse(time.RFC3339, lastSyncTimeStr)
			if err != nil {
				log.Printf("Error parsing last sync time: %v", err)
				syncTimeoutDuration = 1 * time.Minute
			} else {
				// Use adaptive timeout based on last sync time
				hoursSinceSync := time.Since(lastSyncTime).Hours()
				switch {
				case hoursSinceSync > 24:
					syncTimeoutDuration = 2 * time.Minute // Long time since sync
				case hoursSinceSync > 8:
					syncTimeoutDuration = 1 * time.Minute // Moderate time since sync
				default:
					syncTimeoutDuration = 30 * time.Second // Recent sync
				}
			}
		}
	}

	// Use a quick initial sync to get the basic wallet state
	log.Println("Performing quick initial synchronization...")
	initialSyncDone := make(chan struct{})
	go func() {
		defer close(initialSyncDone)
		config.Wallet.SynchronizeRPC(config.ChainClient)
	}()

	// Wait for initial sync to complete or timeout
	// Use a longer timeout for imported wallets
	initialSyncTimeout := 5 * time.Second
	if config.IsImportedWallet {
		initialSyncTimeout = 1 * time.Minute // 1 minute for newly imported wallets
		log.Println("Using extended initial sync timeout for newly imported wallet: 1 minute")
	}

	select {
	case <-time.After(initialSyncTimeout):
		log.Println("Initial sync timeout reached, proceeding with address scanning")
	case <-initialSyncDone:
		log.Println("Initial sync completed, proceeding with address scanning")
	}

	// Retrieve all addresses from the wallet
	allAddresses, err := config.Wallet.AccountAddresses(0)
	if err != nil {
		return fmt.Errorf("failed to get addresses from wallet: %v", err)
	}

	addrCount := len(allAddresses)
	log.Printf("Optimized rescanning for %d addresses", addrCount)
	logger.Info(fmt.Sprintf("Rescanning %d addresses", addrCount))

	// Create a map of addresses for faster lookup
	walletAddressMap := make(map[string]bool, addrCount)
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

	// Create a single quit channel for all rescans
	quit := make(chan struct{})
	defer close(quit)

	// --- Optimized Address Batch Processing ---

	// Calculate optimal batch size
	batchSize := calculateOptimalBatchSize(addrCount)
	log.Printf("Using batch size of %d addresses for optimal scanning", batchSize)

	// Create a thread-safe transaction map with mutex protection
	var txMutex sync.Mutex
	knownTxs := make(map[chainhash.Hash]*btcutil.Tx)

	// Calculate optimal worker count (based on CPU cores, but limit to avoid network saturation)
	numWorkers := runtime.NumCPU()
	if numWorkers > 4 {
		numWorkers = 4 // Cap at 4 to prevent too many concurrent network requests
	}
	log.Printf("Using %d parallel workers for batch processing", numWorkers)

	// Divide addresses into batches
	batches := createAddressBatches(allAddresses, batchSize)
	log.Printf("Created %d address batches for parallel processing", len(batches))

	// Create work channel for batches
	batchChan := make(chan []btcutil.Address, len(batches))

	// Create wait group for workers
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Create error channel to collect worker errors
	errorChan := make(chan error, len(batches))

	// Progress tracking
	var processedAddresses int32
	startTime := time.Now()
	progressTicker := time.NewTicker(10 * time.Second)
	defer progressTicker.Stop()

	// Progress reporting goroutine
	go func() {
		for range progressTicker.C {
			processed := atomic.LoadInt32(&processedAddresses)
			if processed >= int32(addrCount) {
				return
			}

			percent := float64(processed) / float64(addrCount) * 100
			elapsed := time.Since(startTime)
			estimatedTotal := float64(elapsed) / (float64(processed) / float64(addrCount))
			estimatedRemaining := time.Duration(estimatedTotal) - elapsed

			log.Printf("Progress: %.1f%% (%d/%d addresses) - Est. remaining: %v",
				percent, processed, addrCount, estimatedRemaining.Round(time.Second))
		}
	}()

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		workerID := i
		go func() {
			defer wg.Done()

			// Worker-local transaction cache to reduce mutex contention
			localTxCache := make(map[chainhash.Hash]*btcutil.Tx)

			for batch := range batchChan {
				batchSize := len(batch)
				log.Printf("Worker %d processing batch of %d addresses", workerID, batchSize)

				// Process the batch
				err := processBatch(chainSource, batch, config.StartBlock, bestHeight, quit, localTxCache, config.IsImportedWallet)
				if err != nil {
					errorChan <- fmt.Errorf("worker %d batch error: %w", workerID, err)
					// Continue to next batch even on error
				}

				// Update progress counter
				atomic.AddInt32(&processedAddresses, int32(batchSize))
			}

			// Merge worker's transaction cache into global cache
			txMutex.Lock()
			for hash, tx := range localTxCache {
				knownTxs[hash] = tx
			}
			txMutex.Unlock()
		}()
	}

	// Send all batches to the workers
	for _, batch := range batches {
		batchChan <- batch
	}
	close(batchChan)

	// Wait for all workers to complete
	wg.Wait()
	close(errorChan)

	// Check for errors
	errorCount := 0
	for err := range errorChan {
		errorCount++
		log.Printf("Batch scan error: %v", err)
	}

	if errorCount > 0 {
		log.Printf("Completed address scanning with %d errors", errorCount)
		logger.Info(fmt.Sprintf("Address scanning completed with %d errors", errorCount))
	} else {
		log.Println("All address batches scanned successfully")
		logger.Info("Address scanning completed successfully")
	}

	// Log completion stats
	totalTime := time.Since(startTime)
	log.Printf("Address scanning completed in %v (%.1f addresses/sec)",
		totalTime, float64(addrCount)/totalTime.Seconds())

	// Wait for full synchronization to complete or timeout
	log.Printf("Waiting up to %v for final wallet synchronization...", syncTimeoutDuration)
	fullSyncTimeout := time.After(syncTimeoutDuration)
	syncCheckTicker := time.NewTicker(1 * time.Second)
	defer syncCheckTicker.Stop()

FullSyncLoop:
	for {
		select {
		case <-fullSyncTimeout:
			log.Println("Final wallet synchronization timed out, but address scanning completed")
			logger.Info("Wallet synchronization timed out, but address scanning completed")
			break FullSyncLoop
		case <-syncCheckTicker.C:
			if !config.Wallet.SynchronizingToNetwork() {
				log.Println("Final wallet synchronization completed successfully")
				logger.Info("Wallet synchronization completed successfully")
				break FullSyncLoop
			}
		}
	}

	// After rescan and synchronization, update the last sync time
	err = utils.SetLastSyncTime(time.Now())
	if err != nil {
		log.Printf("Error updating last sync time: %v", err)
	}

	// Get the updated balance
	balance, err := config.Wallet.CalculateBalance(1)
	if err != nil {
		return fmt.Errorf("failed to get wallet balance: %v", err)
	}

	log.Printf("Final wallet balance after optimized rescan: %d satoshis", balance)
	log.Printf("Transaction recovery process completed in %v", time.Since(startTime))
	logger.Info("Transaction recovery process completed")

	return nil
}

// getExtraTimeout returns an extended timeout duration based on whether the wallet is newly imported
func getExtraTimeout(isImportedWallet bool) time.Duration {
	if isImportedWallet {
		return 5 * time.Minute // 5 minutes extra for newly imported wallets
	}
	return 2 * time.Minute // 2 minutes extra for regular wallets
}

// calculateOptimalBatchSize determines the optimal batch size based on address count
func calculateOptimalBatchSize(addressCount int) int {
	switch {
	case addressCount > 1000:
		return 40 // Very large wallets: larger batches
	case addressCount > 500:
		return 30 // Large wallets
	case addressCount > 100:
		return 20 // Medium wallets
	case addressCount > 50:
		return 10 // Smaller wallets
	default:
		return 5 // Very small wallets: smaller batches for better responsiveness
	}
}

// createAddressBatches divides addresses into batches of the specified size
func createAddressBatches(addresses []btcutil.Address, batchSize int) [][]btcutil.Address {
	// Calculate how many batches we'll need
	batchCount := (len(addresses) + batchSize - 1) / batchSize

	// Create the batch slices
	batches := make([][]btcutil.Address, 0, batchCount)

	for i := 0; i < len(addresses); i += batchSize {
		end := i + batchSize
		if end > len(addresses) {
			end = len(addresses)
		}
		batches = append(batches, addresses[i:end])
	}

	return batches
}

// processBatch scans a batch of addresses in a single operation
func processBatch(cs *neutrino.RescanChainSource, batch []btcutil.Address, startHeight, endHeight int32,
	quit chan struct{}, knownTxs map[chainhash.Hash]*btcutil.Tx, isImportedWallet bool) error {

	// Skip empty batches
	if len(batch) == 0 {
		return nil
	}

	// Create a channel to track transaction discovery
	txFoundChan := make(chan struct{}, 1)
	txFound := false

	// Log addresses in this batch (only in debug mode to avoid cluttering logs)
	addrStrings := make([]string, 0, len(batch))
	for _, addr := range batch {
		addrStrings = append(addrStrings, addr.String())
	}
	log.Printf("Scanning batch with addresses: %s", strings.Join(addrStrings, ", "))

	// Set up notification handlers with improved logging
	ntfn := rpcclient.NotificationHandlers{
		OnFilteredBlockConnected: func(height int32, header *wire.BlockHeader, txns []*btcutil.Tx) {
			// Progress logging (periodic)
			if height%10000 == 0 || (height-startHeight) < 100 {
				log.Printf("Scanning block %d for batch of %d addresses (%d/%d)",
					height, len(batch), height-startHeight, endHeight-startHeight)
			}

			for _, tx := range txns {
				// Store all transactions so we can calculate inputs correctly
				knownTxs[*tx.Hash()] = tx

				// Check if this transaction is relevant to our batch

				// Check outputs (receiving)
				for _, txOut := range tx.MsgTx().TxOut {
					_, addrs, _, err := txscript.ExtractPkScriptAddrs(txOut.PkScript, &chaincfg.MainNetParams)
					if err != nil {
						continue
					}

					for _, a := range addrs {
						// Check if this address is in our batch
						for _, batchAddr := range batch {
							if a.EncodeAddress() == batchAddr.EncodeAddress() {
								txFound = true
								log.Printf("Transaction %s found for address %s in block %d",
									tx.Hash(), a.EncodeAddress(), height)

								// Signal that we found something
								if !txFound {
									txFound = true
									select {
									case txFoundChan <- struct{}{}:
									default:
									}
								}
							}
						}
					}
				}

				// Check inputs (spending)
				for _, txIn := range tx.MsgTx().TxIn {
					prevTx, ok := knownTxs[txIn.PreviousOutPoint.Hash]
					if !ok {
						continue
					}

					// Skip if the previous output index is out of range
					if int(txIn.PreviousOutPoint.Index) >= len(prevTx.MsgTx().TxOut) {
						continue
					}

					prevTxOut := prevTx.MsgTx().TxOut[txIn.PreviousOutPoint.Index]
					_, addrs, _, err := txscript.ExtractPkScriptAddrs(prevTxOut.PkScript, &chaincfg.MainNetParams)
					if err != nil {
						continue
					}

					for _, a := range addrs {
						// Check if this address is in our batch
						for _, batchAddr := range batch {
							if a.EncodeAddress() == batchAddr.EncodeAddress() {
								txFound = true
								log.Printf("Spending transaction %s found for address %s in block %d",
									tx.Hash(), a.EncodeAddress(), height)

								// Signal that we found something
								if !txFound {
									txFound = true
									select {
									case txFoundChan <- struct{}{}:
									default:
									}
								}
							}
						}
					}
				}
			}
		},
	}

	// Calculate a dynamic timeout based on the block range and wallet type
	blockRange := endHeight - startHeight
	scanTimeoutMinutes := 5 * time.Minute // Default 5 minutes

	if isImportedWallet {
		// For newly imported wallets, use longer timeouts
		scanTimeoutMinutes = 10 * time.Minute // Double the default timeout

		if blockRange > 10000 {
			// For larger ranges in newly imported wallets, scale the timeout with a higher cap
			scanTimeoutMinutes = time.Duration(math.Min(float64(blockRange)/800, 30)) * time.Minute
			// Changed from blockRange/1000 to blockRange/800 and cap from 20 to 30 minutes
		}

		log.Printf("Using extended timeout for newly imported wallet batch: %v", scanTimeoutMinutes)
	} else if blockRange > 10000 {
		// For regular wallets with large block ranges, use the original scaling
		scanTimeoutMinutes = time.Duration(math.Min(float64(blockRange)/1000, 20)) * time.Minute
	}

	// Configure rescan with optimized parameters for batch processing
	rescan := neutrino.NewRescan(
		cs,
		neutrino.StartBlock(&headerfs.BlockStamp{Height: startHeight}),
		neutrino.EndBlock(&headerfs.BlockStamp{Height: endHeight}),
		neutrino.WatchAddrs(batch...), // Pass all addresses in the batch
		neutrino.NotificationHandlers(ntfn),
		neutrino.QuitChan(quit),
		neutrino.QueryOptions(
			neutrino.NumRetries(5),
			neutrino.Timeout(time.Minute*5),
		),
	)

	// Start the rescan process
	log.Printf("Starting batch rescan for %d addresses from block %d to %d (timeout: %v)",
		len(batch), startHeight, endHeight, scanTimeoutMinutes)
	errChan := rescan.Start()

	// Wait for completion or timeout
	select {
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("batch rescan error: %w", err)
		}
		log.Printf("Batch rescan completed for %d addresses", len(batch))
	case <-txFoundChan:
		// If we found transactions, we can be a bit more patient
		log.Printf("Found transactions for batch, extending timeout")
		select {
		case err := <-errChan:
			if err != nil {
				return fmt.Errorf("batch rescan error after finding tx: %w", err)
			}
			log.Printf("Batch rescan fully completed for %d addresses with transactions", len(batch))
		case <-time.After(scanTimeoutMinutes + getExtraTimeout(isImportedWallet)): // Give extra time since we found something, more for newly imported wallets
			log.Printf("Extended batch rescan timeout - continuing with partial results")
		case <-quit:
			return fmt.Errorf("batch rescan was canceled")
		}
	case <-time.After(scanTimeoutMinutes):
		log.Printf("Batch rescan timed out after %v", scanTimeoutMinutes)
		return fmt.Errorf("batch rescan timed out for %d addresses", len(batch))
	case <-quit:
		return fmt.Errorf("batch rescan was canceled")
	}

	return nil
}
