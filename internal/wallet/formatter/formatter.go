package formatter

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/addresses"
	"github.com/Maphikza/btc-wallet-btcsuite.git/lib/rescanner"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/spf13/viper"
)

func PerformRescanAndProcessTransactions(w *wallet.Wallet, chainClient *chain.NeutrinoClient, chainParams *chaincfg.Params, walletName string) error {

	log.Println("Scanning for transactions")

	lastScannedBlockHeight, err := walletstatedb.GetLastScannedBlockHeight()
	if err != nil {
		return fmt.Errorf("error getting last scanned block height: %v", err)
	}

	log.Println("Scanning from block", lastScannedBlockHeight)

	rescanConfig := rescanner.RescanConfig{
		ChainClient: chainClient,
		ChainParams: chainParams,
		StartBlock:  lastScannedBlockHeight,
		Wallet:      w,
	}

	err = rescanner.PerformRescan(rescanConfig)
	if err != nil {
		return fmt.Errorf("error during rescan: %v", err)
	}

	if viper.GetBool("relay_wallet_set") && viper.GetString("wallet_name") == walletName {

		if viper.GetBool("server_mode") {
			FormatAndTransmitTransactions(w, walletName)
			err = FetchAndSendWalletBalance(w, walletName)
			if err != nil {
				return fmt.Errorf("error sending wallet balance: %v", err)
			}

			err = SendReceiveAddressesToBackend(walletName)
			if err != nil {
				return fmt.Errorf("error sending receive addresses: %v", err)
			}
		}
	}

	return nil
}

func FormatAndTransmitTransactions(w *wallet.Wallet, walletName string) {
	log.Println("Processing new transactions...")

	// First, save any new transactions
	err := saveNewTransactions(w, walletName)
	if err != nil {
		log.Printf("Error saving new transactions: %v", err)
		return
	}

	// Then send unsent transactions to backend
	err = sendUnsentTransactions()
	if err != nil {
		log.Printf("Error sending transactions to backend: %v", err)
	}
}

func saveNewTransactions(w *wallet.Wallet, walletName string) error {
	transactions, err := w.ListAllTransactions()
	if err != nil {
		return fmt.Errorf("error listing transactions: %v", err)
	}

	var newTxCount int
	for _, tx := range transactions {
		// Create transaction record
		transaction := &walletstatedb.Transaction{
			TxID:        tx.TxID,
			WalletName:  walletName,
			Address:     tx.TxID + ":" + strconv.FormatUint(uint64(tx.Vout), 10),
			Output:      tx.Address,
			Value:       fmt.Sprintf("%.8f", tx.Amount),
			Date:        time.Unix(tx.Time, 0),
			BlockHeight: tx.BlockHeight,
			Vout:        tx.Vout,
		}

		// Save only if it's a new transaction
		err = walletstatedb.SaveNewTransaction(transaction)
		if err != nil {
			return fmt.Errorf("error saving transaction: %v", err)
		} else {
			newTxCount++
		}
	}

	if newTxCount > 0 {
		log.Printf("Found and saved %d new transactions", newTxCount)
	}

	return nil
}

func sendUnsentTransactions() error {
	// Get unsent transactions
	unsentTxs, err := walletstatedb.GetUnsentTransactions()
	if err != nil {
		return fmt.Errorf("error getting unsent transactions: %v", err)
	}

	if len(unsentTxs) == 0 {
		log.Println("No unsent transactions to process")
		return nil
	}

	// Format transactions for backend
	var formattedTxs []map[string]interface{}
	for _, tx := range unsentTxs {
		formattedTx := map[string]interface{}{
			"wallet_name": tx.WalletName,
			"address":     tx.Address,
			"date":        tx.Date.Format(time.RFC3339),
			"output":      tx.Output,
			"value":       tx.Value,
		}
		formattedTxs = append(formattedTxs, formattedTx)
	}

	// Send to backend
	jsonData, err := json.Marshal(formattedTxs)
	if err != nil {
		return fmt.Errorf("error marshaling transactions: %v", err)
	}

	err = sendToBackend("/api/wallet/transactions", jsonData)
	if err != nil {
		return fmt.Errorf("error sending transactions to backend: %v", err)
	}

	// Clear sent transactions
	err = walletstatedb.ClearUnsentTransactions()
	if err != nil {
		return fmt.Errorf("error clearing unsent transactions: %v", err)
	}

	log.Printf("Successfully sent and cleared %d transactions", len(formattedTxs))
	return nil
}

func FormatTransactions(w *wallet.Wallet, serverwWalletName string) ([]map[string]interface{}, error) {
	transactions, err := w.ListAllTransactions()
	if err != nil {
		return []map[string]interface{}{}, fmt.Errorf("error listing transactions: %v", err)
	}

	var result []map[string]interface{}

	// Assuming wallet name can be accessed like this: w.Name (you might need to adjust based on the actual wallet struct)
	walletName := serverwWalletName

	for _, tx := range transactions {
		transaction := map[string]interface{}{
			"wallet_name": walletName, // Add wallet name to the transaction data
			"address":     tx.TxID + ":" + strconv.FormatUint(uint64(tx.Vout), 10),
			"date":        time.Unix(tx.Time, 0).Format(time.RFC3339), // Format the date to RFC3339 string
			"output":      tx.Address,
			"value":       fmt.Sprintf("%.8f", tx.Amount), // Format the value to 8 decimal places
		}

		result = append(result, transaction)
	}

	return result, nil
}

func SendTransactionsToBackend(transactions []map[string]interface{}) error {
	jsonData, err := json.Marshal(transactions)
	if err != nil {
		return fmt.Errorf("error marshaling transactions: %v", err)
	}

	return sendToBackend("/api/wallet/transactions", jsonData)
}

func FetchAndSendWalletBalance(w *wallet.Wallet, walletName string) error {
	backendURL := viper.GetString("relay_backend_url")
	if backendURL == "" {
		log.Println("Using default value, not config.json values.")
		backendURL = "http://localhost:9002" // Default value
	}

	// Load snapshot
	walletBalance, err := w.CalculateBalance(1)
	if err != nil {
		return fmt.Errorf("error listing unspent: %v", err)
	}

	// Prepare data for sending
	data := map[string]interface{}{
		"wallet_name": walletName, // Include wallet name in the payload
		"balance":     walletBalance,
	}

	// Marshal data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("error marshaling balance data: %v", err)
	}

	return sendToBackend("/api/wallet/balance", jsonData)
}

func SendReceiveAddressesToBackend(walletName string) error {
	// Get unsent addresses instead of all addresses
	unsentAddresses, err := addresses.GetUnsentAddresses()
	if err != nil {
		return fmt.Errorf("error retrieving unsent addresses: %v", err)
	}

	// If there are no unsent addresses, return early
	if len(unsentAddresses) == 0 {
		log.Println("No unsent addresses to send")
		return nil
	}

	// Prepare the data to send
	var addressList []map[string]string
	for i, addr := range unsentAddresses {
		addressList = append(addressList, map[string]string{
			"index":       fmt.Sprintf("%d", i),
			"address":     addr.Address,
			"wallet_name": walletName,
		})
	}

	jsonData, err := json.Marshal(addressList)
	if err != nil {
		return fmt.Errorf("error marshaling addresses: %v", err)
	}

	// Send addresses to backend
	err = sendToBackend("/api/wallet/addresses", jsonData)
	if err != nil {
		return fmt.Errorf("error sending addresses to backend: %v", err)
	}

	// Clear the unsent addresses after successful send
	err = addresses.ClearUnsentAddresses()
	if err != nil {
		return fmt.Errorf("error clearing unsent addresses: %v", err)
	}

	log.Printf("Successfully sent and cleared %d addresses", len(addressList))
	return nil
}

func sendToBackend(endpoint string, data []byte) error {
	backendURL := viper.GetString("relay_backend_url")
	if backendURL == "" {
		log.Println("Using default value, not config.json values.")
		backendURL = "http://localhost:9002" // Default value
	}

	apiKey := viper.GetString("wallet_api_key")
	timestamp := time.Now().UTC().Format(time.RFC3339)
	signature := generateSignature(apiKey, timestamp, data)

	req, err := http.NewRequest("POST", backendURL+endpoint, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Signature", signature)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request to backend: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("backend returned non-OK status: %v", resp.Status)
	}

	log.Println("Data sent successfully to backend")
	return nil
}

func generateSignature(apiKey, timestamp string, data []byte) string {
	message := apiKey + timestamp + string(data)
	h := hmac.New(sha256.New, []byte(apiKey))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}
