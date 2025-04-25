package operations

import (
	"fmt"
	"log"
	"time"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/ipc"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/logger"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/formatter"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/utils"
)

func (s *WalletServer) SyncBlockchain() {
	log.Println("Starting syncing process...")

	logger.Info("Starting syncing process...")

	for i := 0; i < 120; i++ {
		time.Sleep(10 * time.Second)
		bestBlock, err := s.API.ChainService.BestBlock()
		if err != nil {
			log.Printf("Error getting best block: %v", err)
			continue
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

		if s.API.ChainService.IsCurrent() {
			log.Println("Chain is synced!")
			logger.Info("Chain is synced!")
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
			s.SyncBlockchain()

			s.API.ChainClient.Notifications()

			s.API.Wallet.SynchronizeRPC(s.API.ChainClient)

			formatter.PerformRescanAndProcessTransactions(s.API.Wallet, s.API.ChainClient, s.API.ChainParams, s.API.Name)

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

				s.SyncBlockchain()

				s.API.ChainClient.Notifications()

				s.API.Wallet.SynchronizeRPC(s.API.ChainClient)

				formatter.PerformRescanAndProcessTransactions(s.API.Wallet, s.API.ChainClient, s.API.ChainParams, s.API.Name)
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
