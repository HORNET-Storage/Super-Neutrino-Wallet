package wallet

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/ipc"
	transaction "github.com/Maphikza/btc-wallet-btcsuite.git/lib/transaction"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/lightninglabs/neutrino"
	"github.com/spf13/viper"
)

var (
	engaged     bool
	exitMutex   sync.Mutex
	exiting     bool
	transacting bool
)

func (s *WalletServer) serverLoop() error {

	syncTicker := time.NewTicker(syncInterval)

	ipcServer, err := ipc.NewServer()
	if err != nil {
		return fmt.Errorf("failed to create IPC server: %v", err)
	}
	defer ipcServer.Close()

	err = setWalletSync(true)
	if err != nil {
		log.Printf("Error setting wallet sync state: %v", err)
	}

	go s.handleIPCCommands(ipcServer)

	userCommandChannel := make(chan string)
	go listenForUserCommands(userCommandChannel)

	for {
		select {
		case <-syncTicker.C:
			if !transacting {
				engaged = true

				err = setWalletSync(false)
				if err != nil {
					log.Printf("Error setting wallet sync state: %v", err)
				}

				s.syncBlockchain()

				s.API.ChainClient.Notifications()

				s.API.Wallet.SynchronizeRPC(s.API.ChainClient)

				PerformRescanAndProcessTransactions(s.API.Wallet, s.API.ChainClient, s.API.ChainParams, s.API.Name)
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
				err = setWalletSync(true)
				if err != nil {
					log.Printf("Error setting wallet sync state: %v", err)
				}

			}

		case command := <-userCommandChannel:
			if err := s.handleCommand(command); err != nil {
				log.Printf("Error handling command: %v", err)
			}

			if exiting {
				return nil // Exit the server loop if we're in the process of shutting down
			}
		}
	}
}

func setWalletSync(synced bool) error {
	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}

	if !viper.IsSet("wallet_synced") {
		viper.Set("wallet_synced", synced)
	} else {
		viper.Set("wallet_synced", synced)
	}

	err = viper.WriteConfig()
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

func setWalletLive(live bool) error {
	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}

	if !viper.IsSet("wallet_live") {
		viper.Set("wallet_live", live)
	} else {
		viper.Set("wallet_live", live)
	}

	err = viper.WriteConfig()
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

func (s *WalletServer) handleCommand(command string) error {
	if s.API.HttpMode {
		return fmt.Errorf("terminal commands are not available in HTTP mode")
	}

	switch command {
	case "tx":
		return s.performTransaction()
	case "exit":
		return s.exitWallet()
	case "seed-view":
		return s.viewSeedPhrase()
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func (s *WalletServer) handleIPCCommands(server *ipc.Server) {
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
			result, err = s.handleGetWalletBalance()
		case "estimate-transaction-size":
			result, err = s.handleEstimateTransactionSize(cmd.Args)
		case "get-transaction-history":
			result, err = s.handleGetTransactionHistory()
		default:
			err = fmt.Errorf("unknown command: %s", cmd.Command)
		}
		log.Println("CMD Results: ", result)
		response := ipc.Response{ID: cmd.ID, Error: err, Result: result}
		server.SendResponse(cmd.ID, response)
	}
}

func listenForUserCommands(commandChannel chan<- string) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		if !engaged {
			fmt.Println("\nAvailable commands:")
			fmt.Println("- 'tx': Enter transaction context")
			fmt.Println("- 'seed-view': View seed phrase")
			fmt.Println("- 'exit': Close the wallet")
			fmt.Print("\nEnter command: ")
			scanner.Scan()
			command := strings.TrimSpace(scanner.Text())

			commandChannel <- command
		}
		time.Sleep(100 * time.Millisecond) // Add a small delay to reduce CPU usage
	}
}

func (s *WalletServer) handleGetWalletBalance() (interface{}, error) {
	balance, err := GetWalletBalance(s.API.Wallet)
	if err != nil {
		return nil, err
	}
	log.Printf("Wallet balance retrieved: %d\n", balance)
	return map[string]int64{"balance": balance}, nil
}

func (s *WalletServer) handleEstimateTransactionSize(args []string) (interface{}, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("invalid number of arguments for estimate-transaction-size")
	}
	spendAmount, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid spend amount: %v", err)
	}
	recipientAddress := args[1]
	feeRate, err := strconv.Atoi(args[2])
	if err != nil {
		return nil, fmt.Errorf("invalid fee rate: %v", err)
	}

	size, err := EstimateTransactionSize(s.API.Wallet, spendAmount, recipientAddress, feeRate)
	if err != nil {
		return nil, err
	}
	return map[string]int{"size": size}, nil
}

func (s *WalletServer) handleGetTransactionHistory() (interface{}, error) {
	history, err := GetTransactionHistory(s.API.Wallet, s.API.Name)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"transactions": history}, nil
}

func (s *WalletServer) viewSeedPhrase() error {
	engaged = true
	defer func() { engaged = false }()

	return viewSeedPhrase()
}

func (s *WalletServer) exitWallet() error {
	exitMutex.Lock()
	defer exitMutex.Unlock()

	if exiting {
		return nil // Exit is already in progress, do nothing
	}

	engaged = true
	defer func() { engaged = false }()

	fmt.Print("Are you sure you want to exit? (y/n): ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	confirmation := strings.ToLower(strings.TrimSpace(scanner.Text()))

	if confirmation == "y" {
		exiting = true
		err := setWalletLive(false)
		if err != nil {
			log.Printf("Error setting wallet live state: %v", err)
		}
		fmt.Println("Initiating graceful shutdown...")
		if err := gracefulShutdown(); err != nil {
			return fmt.Errorf("error during shutdown: %v", err)
		}
	} else {
		fmt.Println("Shutdown cancelled.")
	}
	return nil
}

func (s *WalletServer) performTransaction() error {
	// Implement your transaction logic here
	// This is a placeholder for your existing transaction code
	log.Println("Performing transaction...")

	scanner := bufio.NewScanner(os.Stdin)
	enableRBF := true
	transactionComplete := false

	for !transactionComplete {
		engaged = true
		fmt.Println("Choose an action:")
		fmt.Println("1. New transaction")
		fmt.Println("2. RBF (Replace-By-Fee) transaction")
		fmt.Println("3. New transaction with file hash")
		fmt.Println("4. Get Receive Address")
		fmt.Println("5. Exit tx")
		fmt.Print("\nEnter your choice (1, 2, 3, 4, or 5): ")

		scanner.Scan()
		choice := strings.TrimSpace(scanner.Text())

		switch choice {
		case "1":
			transacting = true
			log.Println("Creating new transaction")
			// Ask for recipient address
			fmt.Print("Enter the recipient address: ")
			scanner.Scan()
			recipientAddress := strings.TrimSpace(scanner.Text())

			// Validate the recipient address
			_, err := btcutil.DecodeAddress(recipientAddress, s.API.Wallet.ChainParams())
			if err != nil {
				log.Printf("Invalid recipient address: %v", err)
				continue
			}

			// Ask for spend amount
			fmt.Print("Enter the spend amount (satoshis): ")
			scanner.Scan()
			var spendAmount int64
			_, err = fmt.Sscan(scanner.Text(), &spendAmount)
			if err != nil {
				log.Printf("Error reading spend amount: %v", err)
				continue
			}

			// Call the transaction creation function with the new recipient address parameter
			txid, verified, err := transaction.CheckBalanceAndCreateTransaction(s.API.Wallet, s.API.ChainClient.CS, enableRBF, spendAmount, recipientAddress, s.API.PrivPass)
			if err != nil {
				log.Println("Closing in 1 minute...")
				time.Sleep(1 * time.Minute)
				return fmt.Errorf("error creating or broadcasting transaction: %v", err)
			}

			if verified {
				log.Printf("Transaction successfully broadcasted with TXID: %s", txid)
			} else {
				log.Printf("Transaction with TXID: %s failed.", txid)
			}

			transactionComplete = true

		case "2":
			transacting = true
			var mempoolSpaceConfig = transaction.ElectrumConfig{
				ServerAddr: "electrum.blockstream.info:50002",
				UseSSL:     true,
			}
			client, err := transaction.CreateElectrumClient(mempoolSpaceConfig)
			if err != nil {
				log.Fatalf("Failed to create Electrum client: %v", err)
			}
			log.Println("Performing RBF transaction")
			fmt.Print("Enter the original transaction ID: ")
			scanner.Scan()
			originalTxID := strings.TrimSpace(scanner.Text())

			fmt.Print("Enter new fee rate (sat/vB): ")
			scanner.Scan()
			var newFeeRate int64
			_, err = fmt.Sscan(scanner.Text(), &newFeeRate)
			if err != nil {
				log.Printf("Error reading new fee rate: %v", err)
				continue
			}

			newTxID, verified, err := transaction.ReplaceTransactionWithHigherFee(s.API.Wallet, s.API.ChainClient.CS, originalTxID, newFeeRate, client, s.API.PrivPass)
			if err != nil {
				log.Println("Closing in 1 minute...")
				time.Sleep(1 * time.Minute)
				return fmt.Errorf("error performing RBF transaction: %v", err)
			}

			if verified {
				log.Printf("RBF transaction successfully broadcasted with new TXID: %s", newTxID)
			} else {
				log.Printf("Transaction with TXID: %s failed.", newTxID)
			}

			transactionComplete = true

		case "3":
			transacting = true
			log.Println("Creating new transaction with file hash")
			// Ask for recipient address
			fmt.Print("Enter the recipient address: ")
			scanner.Scan()
			recipientAddress := strings.TrimSpace(scanner.Text())

			// Validate the recipient address
			_, err := btcutil.DecodeAddress(recipientAddress, s.API.Wallet.ChainParams())
			if err != nil {
				log.Printf("Invalid recipient address: %v", err)
				continue
			}

			// Ask for spend amount
			fmt.Print("Enter the spend amount (satoshis): ")
			scanner.Scan()
			var spendAmount int64
			_, err = fmt.Sscan(scanner.Text(), &spendAmount)
			if err != nil {
				log.Printf("Error reading spend amount: %v", err)
				continue
			}

			// Ask for file path
			fmt.Print("Enter the path to the file you want to hash: ")
			scanner.Scan()
			filePath := strings.TrimSpace(scanner.Text())

			// Hash the file
			fileHash, err := hashFile(filePath)
			if err != nil {
				log.Printf("Error hashing file: %v", err)
				continue
			}

			// Display the file hash and ask for confirmation
			fmt.Printf("File hash: %s\n", fileHash)
			fmt.Print("Do you want to proceed with this hash? (y/n): ")
			scanner.Scan()
			confirmation := strings.ToLower(strings.TrimSpace(scanner.Text()))

			if confirmation != "y" && confirmation != "yes" {
				log.Println("Transaction cancelled.")
				continue
			}

			// Call the transaction creation function with the new recipient address and file hash parameters
			txid, verified, err := transaction.CreateTransactionWithHash(s.API.Wallet, s.API.ChainClient.CS, enableRBF, spendAmount, recipientAddress, fileHash, s.API.PrivPass)
			if err != nil {
				log.Println("Closing in 1 minute...")
				time.Sleep(1 * time.Minute)
				return fmt.Errorf("error creating or broadcasting transaction with file hash: %v", err)
			}

			if verified {
				log.Printf("Transaction with file hash successfully broadcasted with TXID: %s", txid)
			} else {
				log.Printf("Transaction with TXID: %s failed.", txid)
			}

			transactionComplete = true
		case "4":
			transacting = true
			_, err := walletstatedb.PrintAndCopyReceiveAddresses()
			if err != nil {
				return fmt.Errorf("error getting receive address: %v", err)
			}
			transactionComplete = true

		case "5":
			log.Println("Exiting tx...")
			transactionComplete = true
		default:
			log.Println("Invalid choice. Please enter 1, 2, 3, 4, or 5.")
		}
		engaged = false
		transacting = false
	}

	return nil
}

func (s *WalletServer) NewTransactionAPI(recipient string, amountStr, feeRateStr string) (map[string]interface{}, error) {
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %v", err)
	}

	feeRate, err := strconv.ParseInt(feeRateStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid fee rate: %v", err)
	}

	txHash, verified, err := transaction.HttpCheckBalanceAndCreateTransaction(s.API.Wallet, s.API.ChainClient.CS, true, amount, recipient, s.API.PrivPass, int(feeRate))
	if err != nil {
		return nil, fmt.Errorf("transaction failed: %v", err)
	}

	return map[string]interface{}{
		"txHash":   txHash.String(),
		"verified": verified,
	}, nil
}

func (s *WalletServer) RBFTransactionAPI(originalTxID, newFeeRateStr string) (map[string]interface{}, error) {
	newFeeRate, err := strconv.ParseInt(newFeeRateStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid fee rate: %v", err)
	}

	client, err := transaction.CreateElectrumClient(transaction.ElectrumConfig{
		ServerAddr: "electrum.blockstream.info:50002",
		UseSSL:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Electrum client: %v", err)
	}
	defer client.Shutdown()

	newTxID, verified, err := transaction.ReplaceTransactionWithHigherFee(s.API.Wallet, s.API.ChainClient.CS, originalTxID, newFeeRate, client, s.API.PrivPass)
	if err != nil {
		return nil, fmt.Errorf("RBF transaction failed: %v", err)
	}

	return map[string]interface{}{
		"newTxID":  newTxID.String(),
		"verified": verified,
	}, nil
}

func (s *WalletServer) syncBlockchain() {
	log.Println("Starting syncing process...")

	for i := 0; i < 120; i++ {
		time.Sleep(10 * time.Second)
		bestBlock, err := s.API.ChainService.BestBlock()
		if err != nil {
			log.Printf("Error getting best block: %v", err)
			continue
		}
		log.Printf("Current block height: %d", bestBlock.Height)

		currentHash, err := s.API.ChainService.GetBlockHash(int64(bestBlock.Height))
		if err != nil {
			log.Printf("Error getting current block hash: %v", err)
		} else {
			log.Printf("Current block hash: %s", currentHash.String())
		}

		peers := s.API.ChainService.Peers()
		log.Printf("Connected peers: %d", len(peers))

		if s.API.ChainService.IsCurrent() {
			log.Println("Chain is synced!")
			break
		}
	}
}

func initialChainServiceSync(chainService *neutrino.ChainService) {
	log.Println("Starting initial syncing process...")

	for i := 0; i < 120; i++ {
		time.Sleep(10 * time.Second)
		bestBlock, err := chainService.BestBlock()
		if err != nil {
			log.Printf("Error getting best block: %v", err)
			continue
		}
		log.Printf("Current block height: %d", bestBlock.Height)

		currentHash, err := chainService.GetBlockHash(int64(bestBlock.Height))
		if err != nil {
			log.Printf("Error getting current block hash: %v", err)
		} else {
			log.Printf("Current block hash: %s", currentHash.String())
		}

		peers := chainService.Peers()
		log.Printf("Connected peers: %d", len(peers))

		if chainService.IsCurrent() {
			log.Println("Chain is synced!")
			break
		}
	}
}

func initializeChainClient(chainParams *chaincfg.Params, chainService *neutrino.ChainService) (*chain.NeutrinoClient, error) {
	chainClient := chain.NewNeutrinoClient(chainParams, chainService)
	err := chainClient.Start()
	if err != nil {
		return nil, fmt.Errorf("error starting chain client: %v", err)
	}
	return chainClient, nil
}
