package wallet

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/ipc"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/joho/godotenv"
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
		case "get-receive-addresses":
			result, err = s.handleGetReceiveAddresses()
		case "exit":
			err = s.exitWalletCMD()
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

func (s *WalletServer) viewSeedPhrase() error {
	engaged = true
	defer func() { engaged = false }()

	return viewSeedPhrase()
}

func (s *WalletServer) handleGetReceiveAddresses() (interface{}, error) {
	// Retrieve receive and change addresses
	receiveAddresses, _, err := walletstatedb.RetrieveAddresses()
	if err != nil {
		return nil, err
	}

	// Convert []btcutil.Address to []string
	receiveAddressStrings := make([]string, len(receiveAddresses))
	for i, addr := range receiveAddresses {
		receiveAddressStrings[i] = addr.String() // Convert each btcutil.Address to its string representation
	}

	log.Printf("Receive addresses retrieved: %v\n", receiveAddressStrings)
	return map[string][]string{"addresses": receiveAddressStrings}, nil
}

func viewSeedPhrase() error {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("Enter the name of the wallet to view seed phrase: ")
	scanner.Scan()
	walletName := strings.TrimSpace(scanner.Text())

	envFile := filepath.Join(walletDir, walletName+".env")
	err := godotenv.Load(envFile)
	if err != nil {
		return fmt.Errorf("error loading wallet file: %v", err)
	}

	encryptedSeedPhrase := os.Getenv("ENCRYPTED_SEED_PHRASE")
	if encryptedSeedPhrase == "" {
		return fmt.Errorf("encrypted seed phrase not found")
	}

	fmt.Print("Enter your wallet password: ")
	reader := bufio.NewReader(os.Stdin)
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	seedPhrase, err := Decrypt(encryptedSeedPhrase, password)
	if err != nil {
		return fmt.Errorf("error decrypting seed phrase: %v", err)
	}

	fmt.Println("Your seed phrase is:")
	fmt.Println(seedPhrase)
	fmt.Println("Please ensure you store this securely and never share it with anyone.")

	return nil
}
