package operations

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/ipc"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/logger"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/formatter"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/utils"
)

func (s *WalletServer) SyncBlockchain(ipcServer *ipc.Server) {
	log.Println("Starting syncing process...")
	logger.Info("Starting syncing process...")

	// Send initial progress update
	initialUpdate := ipc.SyncProgressUpdate{
		Type:         "sync_progress",
		Progress:     0.0,
		CurrentBlock: 0,
		TargetBlock:  800000, // Approximate target - will be updated with real data
		ChainSynced:  false,
		ScanProgress: 0.0,
		Stage:        "chain_sync",
	}

	if ipcServer != nil {
		ipcServer.BroadcastProgress(initialUpdate)
	}

	// Also output to stdout for CLI clients
	outputProgressToStdout(initialUpdate)

	for i := 0; i < 120; i++ {
		time.Sleep(2 * time.Second) // Reduced from 10s to 2s for more frequent updates
		bestBlock, err := s.API.ChainService.BestBlock()
		if err != nil {
			log.Printf("Error getting best block: %v", err)
			continue
		}

		// Get chain info to determine target height
		var targetHeight int32 = 800000 // Default fallback
		var progress float64 = 0.0

		// For now, use a reasonable estimate for target height
		// In a real implementation, you might get this from the chain service
		if bestBlock.Height > 0 {
			// Estimate progress based on known blockchain height (approximate)
			targetHeight = 870000 // Current approximate Bitcoin block height
			progress = float64(bestBlock.Height) / float64(targetHeight) * 100
		}

		formattedHeight := utils.FormatBlockHeight(bestBlock.Height)
		log.Printf("Current block height: %s", formattedHeight)
		logger.Info("Current block height: ", formattedHeight)

		// Log the current block number instead of hash
		log.Printf("Current block number: %s", formattedHeight)
		logger.Info("Current block number: ", formattedHeight)

		peers := s.API.ChainService.Peers()
		log.Printf("Connected peers: %d", len(peers))
		logger.Info("Connected peers: ", len(peers))

		// Send progress update
		progressUpdate := ipc.SyncProgressUpdate{
			Type:         "sync_progress",
			Progress:     progress,
			CurrentBlock: bestBlock.Height,
			TargetBlock:  targetHeight,
			ChainSynced:  false,
			ScanProgress: 0.0,
			Stage:        "chain_sync",
		}

		if ipcServer != nil {
			ipcServer.BroadcastProgress(progressUpdate)
		}

		// Also output to stdout for CLI clients
		outputProgressToStdout(progressUpdate)

		if s.API.ChainService.IsCurrent() {
			log.Println("Chain is synced!")
			logger.Info("Chain is synced!")

			// Send chain synced update
			synedUpdate := ipc.SyncProgressUpdate{
				Type:         "sync_progress",
				Progress:     100.0,
				CurrentBlock: bestBlock.Height,
				TargetBlock:  targetHeight,
				ChainSynced:  true,
				ScanProgress: 0.0,
				Stage:        "chain_sync",
			}

			if ipcServer != nil {
				ipcServer.BroadcastProgress(synedUpdate)
			}

			// Also output to stdout for CLI clients
			outputProgressToStdout(synedUpdate)
			break
		}
	}
}

func (s *WalletServer) StartSyncProcess() {
	syncTicker := time.NewTicker(baseSyncInterval)
	defer syncTicker.Stop() // Ensure the ticker is properly stopped when done

	for range syncTicker.C {
		if !transacting {
			log.Println("Starting periodic sync process...")
			engaged = true
			s.SyncBlockchain(nil) // No IPC server for HTTP mode

			s.API.ChainClient.Notifications()

			s.API.Wallet.SynchronizeRPC(s.API.ChainClient)

			formatter.PerformRescanAndProcessTransactions(s.API.Wallet, s.API.ChainClient, s.API.ChainParams, s.API.Name, nil)

			// Update the last scanned block height
			_, lastScannedHeight, err := s.API.ChainClient.GetBestBlock()
			if err != nil {
				log.Printf("Error getting last scanned block height: %v", err)
			} else {
				err = walletstatedb.UpdateLastScannedBlockHeight(lastScannedHeight)
				if err != nil {
					log.Printf("Error updating last scanned block height: %v", err)
				} else {
					log.Printf("Updated last scanned block height to %d", lastScannedHeight)
				}
			}

			engaged = false
			log.Println("Sync process completed.")
			logger.Info("Sync process completed.")
		}
	}
}

func (s *WalletServer) serverLoop() error {

	syncTicker := time.NewTicker(baseSyncInterval)

	ipcServer, err := ipc.NewServer()
	if err != nil {
		return fmt.Errorf("failed to create IPC server: %v", err)
	}
	defer ipcServer.Close()

	err = utils.SetWalletSync(true)
	if err != nil {
		log.Printf("Error setting wallet sync state: %v", err)
	}

	logger.Info("Wallet synced")

	go s.HandleIPCCommands(ipcServer)

	userCommandChannel := make(chan string)
	go ListenForUserCommands(userCommandChannel)

	for {
		select {
		case <-syncTicker.C:
			if !transacting {
				engaged = true

				err = utils.SetWalletSync(false)
				if err != nil {
					log.Printf("Error setting wallet sync state: %v", err)
				}

				s.SyncBlockchain(ipcServer)

				s.API.ChainClient.Notifications()

				s.API.Wallet.SynchronizeRPC(s.API.ChainClient)

				formatter.PerformRescanAndProcessTransactions(s.API.Wallet, s.API.ChainClient, s.API.ChainParams, s.API.Name, ipcServer)
				// Update the last scanned block height
				_, lastScannedHeight, err := s.API.ChainClient.GetBestBlock()
				if err != nil {
					log.Printf("Error getting last scanned block height: %v", err)
				} else {
					err = walletstatedb.UpdateLastScannedBlockHeight(lastScannedHeight)
					if err != nil {
						log.Printf("Error updating last scanned block height: %v", err)
					} else {
						log.Printf("Updated last scanned block height to %d", lastScannedHeight)
					}
				}
				engaged = false
				err = utils.SetWalletSync(true)
				if err != nil {
					log.Printf("Error setting wallet sync state: %v", err)
				}

			}

		case command := <-userCommandChannel:
			if err := s.HandleCommand(command); err != nil {
				log.Printf("Error handling command: %v", err)
			}

			if exiting {
				return nil // Exit the server loop if we're in the process of shutting down
			}
		}
	}
}

func (s *WalletServer) HandleIPCCommands(server *ipc.Server) {
	for cmd := range server.Commands() {
		var result interface{}
		var err error

		switch cmd.Command {
		case "new-transaction":
			recipient := cmd.Args[0]
			amount := cmd.Args[1]
			feeRate := cmd.Args[2]
			result, err = s.NewTransactionAPI(recipient, amount, feeRate)
		case "rbf-transaction":
			originalTxID := cmd.Args[0]
			newFeeRate := cmd.Args[1]
			result, err = s.RBFTransactionAPI(originalTxID, newFeeRate)
		case "get-wallet-balance":
			result, err = s.HandleGetWalletBalance()
		case "estimate-transaction-size":
			result, err = s.HandleEstimateTransactionSize(cmd.Args)
		case "get-transaction-history":
			result, err = s.HandleGetTransactionHistory()
		case "get-receive-addresses":
			result, err = s.HandleGetReceiveAddresses()
		case "exit":
			err = s.ExitWalletCMD()
		default:
			err = fmt.Errorf("unknown command: %s", cmd.Command)
		}
		log.Println("CMD Results: ", result)
		response := ipc.Response{ID: cmd.ID, Error: err, Result: result}
		server.SendResponse(cmd.ID, response)
	}
}

// outputProgressToStdout sends progress updates to stdout for CLI clients (like Electron)
func outputProgressToStdout(update ipc.SyncProgressUpdate) {
	// Output JSON format for easy parsing by Electron app
	jsonData, err := json.Marshal(update)
	if err != nil {
		log.Printf("Error marshaling progress update for stdout: %v", err)
		return
	}

	// Output to stdout with a newline for line-by-line parsing
	fmt.Fprintf(os.Stdout, "%s\n", string(jsonData))
}
