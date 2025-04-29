package formatter

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/addresses"
	utils "github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/utils"
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

	// Check if this is a newly imported wallet by reading the flag from config
	isImportedWallet := utils.IsNewlyImportedWallet()

	if isImportedWallet {
		log.Println("Detected newly imported wallet - using extended processing timeouts")
	} else {
		log.Println("Using normal processing timeouts for existing wallet")
	}

	rescanConfig := rescanner.RescanConfig{
		ChainClient:      chainClient,
		ChainParams:      chainParams,
		StartBlock:       lastScannedBlockHeight,
		Wallet:           w,
		IsImportedWallet: isImportedWallet,
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
	// Get unsent transactions using the SentToBackend field
	unsentTxs, err := walletstatedb.GetUnsentTransactionsUsingSentToBackend()
	if err != nil {
		log.Printf("Error getting unsent transactions using SentToBackend: %v, falling back to old method", err)
		// Fallback to old method
		unsentTxs, err = walletstatedb.GetUnsentTransactions()
		if err != nil {
			return fmt.Errorf("error getting unsent transactions: %v", err)
		}
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

	responseBody, err := sendToBackend("/api/wallet/transactions", jsonData)
	if err != nil {
		return fmt.Errorf("error sending transactions to backend: %v", err)
	}

	// Parse response
	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return fmt.Errorf("error parsing backend response: %v", err)
	}

	if result.Status != "success" {
		return fmt.Errorf("backend returned non-success status: %s - %s", result.Status, result.Message)
	}

	// Mark transactions as sent using the SentToBackend field
	err = walletstatedb.MarkTransactionsAsSent()
	if err != nil {
		log.Printf("Error marking transactions as sent using SentToBackend: %v, falling back to old method", err)
		// Fallback to old method
		err = walletstatedb.ClearUnsentTransactions()
		if err != nil {
			return fmt.Errorf("error clearing unsent transactions: %v", err)
		}
	}

	log.Printf("Successfully sent and cleared %d transactions: %s", len(formattedTxs), result.Message)
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

	responseBody, err := sendToBackend("/api/wallet/transactions", jsonData)
	if err != nil {
		return fmt.Errorf("error sending transactions to backend: %v", err)
	}

	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return fmt.Errorf("error parsing backend response: %v", err)
	}

	if result.Status != "success" {
		return fmt.Errorf("backend returned non-success status: %s - %s", result.Status, result.Message)
	}

	log.Printf("Successfully sent transactions: %s", result.Message)
	return nil
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
		"wallet_name": walletName,
		"balance":     walletBalance,
	}

	// Marshal data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("error marshaling balance data: %v", err)
	}

	responseBody, err := sendToBackend("/api/wallet/balance", jsonData)
	if err != nil {
		return fmt.Errorf("error sending balance to backend: %v", err)
	}

	var result struct {
		Message string `json:"message"`
		Balance string `json:"balance"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return fmt.Errorf("error parsing backend response: %v", err)
	}

	if result.Message != "success" {
		return fmt.Errorf("backend returned non-success message: %s", result.Message)
	}

	log.Printf("Successfully sent wallet balance: %s", result.Balance)
	return nil
}

func SendReceiveAddressesToBackend(walletName string) error {
	unsentAddresses, err := addresses.GetUnsentAddresses()
	if err != nil {
		return fmt.Errorf("error retrieving unsent addresses: %v", err)
	}

	if len(unsentAddresses) == 0 {
		log.Println("No unsent addresses to send")
		return nil
	}

	var addressList []map[string]interface{}
	for i, addr := range unsentAddresses {
		addressList = append(addressList, map[string]interface{}{
			"index":       fmt.Sprintf("%d", i),
			"address":     addr.Address,
			"wallet_name": walletName,
		})
	}

	jsonData, err := json.Marshal(addressList)
	if err != nil {
		return fmt.Errorf("error marshaling addresses: %v", err)
	}

	responseBody, err := sendToBackend("/api/wallet/addresses", jsonData)
	if err != nil {
		return fmt.Errorf("backend request failed: %v", err)
	}

	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return fmt.Errorf("error parsing backend response: %v", err)
	}

	if result.Status != "success" {
		return fmt.Errorf("backend returned non-success status: %s - %s", result.Status, result.Message)
	}

	// Only clear addresses after confirming success
	if err := addresses.ClearUnsentAddresses(); err != nil {
		return fmt.Errorf("error clearing unsent addresses: %v", err)
	}

	log.Printf("Successfully sent and cleared %d addresses: %s", len(addressList), result.Message)
	return nil
}

func sendToBackend(endpoint string, data []byte) ([]byte, error) {
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
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Signature", signature)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request to backend: %v", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return responseBody, fmt.Errorf("backend returned non-OK status: %v, body: %s", resp.Status, string(responseBody))
	}

	log.Println("Data sent successfully to backend")
	return responseBody, nil
}

func generateSignature(apiKey, timestamp string, data []byte) string {
	message := apiKey + timestamp + string(data)
	h := hmac.New(sha256.New, []byte(apiKey))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}
