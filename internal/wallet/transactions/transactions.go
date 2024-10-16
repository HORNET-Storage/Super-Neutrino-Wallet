package transactions

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/api"
	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/utils"
	transaction "github.com/Maphikza/btc-wallet-btcsuite.git/lib/transaction"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/spf13/viper"
)

type WalletServer struct {
	API *api.API
}

var (
	engaged     bool
	exitMutex   sync.Mutex
	exiting     bool
	transacting bool
)

func (s *WalletServer) ExitWalletCMD() error {
	exitMutex.Lock()
	defer exitMutex.Unlock()

	if exiting {
		return nil // Exit is already in progress, do nothing
	}

	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}

	engaged = true
	defer func() { engaged = false }()

	// Set wallet_synced and wallet_live to false before initiating the shutdown
	err = utils.SetWalletSync(false)
	if err != nil {
		log.Printf("Error setting wallet synced state to false: %v", err)
	}

	err = utils.SetWalletLive(false)
	if err != nil {
		log.Printf("Error setting wallet live state: %v", err)
	}

	exiting = true
	fmt.Println("Initiating graceful shutdown...")

	if err := utils.GracefulShutdown(); err != nil {
		return fmt.Errorf("error during shutdown: %v", err)
	}

	return nil
}

func (s *WalletServer) ExitWallet() error {
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
		err := utils.SetWalletLive(false)
		if err != nil {
			log.Printf("Error setting wallet live state: %v", err)
		}
		fmt.Println("Initiating graceful shutdown...")
		if err := utils.GracefulShutdown(); err != nil {
			return fmt.Errorf("error during shutdown: %v", err)
		}
	} else {
		fmt.Println("Shutdown cancelled.")
	}
	return nil
}

func (s *WalletServer) PerformTransaction() error {
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

func (s *WalletServer) HandleGetWalletBalance() (interface{}, error) {
	balance, err := GetWalletBalance(s.API.Wallet)
	if err != nil {
		return nil, err
	}
	log.Printf("Wallet balance retrieved: %d\n", balance)
	return map[string]int64{"balance": balance}, nil
}

func (s *WalletServer) HandleEstimateTransactionSize(args []string) (interface{}, error) {
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

func (s *WalletServer) HandleGetTransactionHistory() (interface{}, error) {
	history, err := GetTransactionHistory(s.API.Wallet, s.API.Name)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"transactions": history}, nil
}

func (s *WalletServer) NewTransactionAPI(recipient string, amountStr, feeRateStr string) (map[string]interface{}, error) {
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil {
		return map[string]interface{}{"error": fmt.Sprintf("invalid amount: %v", err)}, nil
	}

	feeRate, err := strconv.ParseInt(feeRateStr, 10, 64)
	if err != nil {
		return map[string]interface{}{"error": fmt.Sprintf("invalid fee rate: %v", err)}, nil
	}

	txHash, verified, err := transaction.HttpCheckBalanceAndCreateTransaction(s.API.Wallet, s.API.ChainClient.CS, true, amount, recipient, s.API.PrivPass, int(feeRate))
	if err != nil {
		return map[string]interface{}{"error": fmt.Sprintf("transaction failed: %v", err)}, nil
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

func EstimateTransactionSize(w *wallet.Wallet, spendAmount int64, recipientAddress string, feeRate int) (int, error) {
	return transaction.HttpCalculateTransactionSize(w, spendAmount, recipientAddress, feeRate)
}

func GetTransactionHistory(w *wallet.Wallet, walletName string) ([]map[string]interface{}, error) {
	transactions, err := w.ListAllTransactions()
	if err != nil {
		return nil, fmt.Errorf("error listing transactions: %v", err)
	}

	var result []map[string]interface{}

	for _, tx := range transactions {
		transaction := map[string]interface{}{
			"txid":   tx.TxID,
			"date":   time.Unix(tx.Time, 0).Format(time.RFC3339),
			"amount": fmt.Sprintf("%.8f", tx.Amount),
		}
		result = append(result, transaction)
	}

	return result, nil
}

func hashFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("error opening file: %v", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("error hashing file: %v", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func GetWalletBalance(w *wallet.Wallet) (int64, error) {
	balance, err := w.CalculateBalance(1) // Use 1 confirmation
	if err != nil {
		return 0, fmt.Errorf("error calculating balance: %v", err)
	}
	return int64(balance), nil
}
