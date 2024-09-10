package wallet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/lib/rescanner"
	"github.com/Maphikza/btc-wallet-btcsuite.git/lib/types"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/spf13/viper"
)

var AppConfig types.Config

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

	FormatAndTransmitTransactions(w, walletName)
	err = FetchAndSendWalletBalance(w, walletName)
	if err != nil {
		return fmt.Errorf("error sending wallet balance: %v", err)
	}

	err = SendReceiveAddressesToBackend(walletName)
	if err != nil {
		return fmt.Errorf("error sending receive addresses: %v", err)
	}

	return nil
}

func FormatAndTransmitTransactions(w *wallet.Wallet, walletName string) {
	log.Println("Listing transactions after rescan...")

	formattedTransactions, err := FormatTransactions(w, walletName)
	if err != nil {
		log.Printf("couldn't format transactions %v", err)
	}

	// Print transactions for verification
	for _, tx := range formattedTransactions {
		log.Printf("WTxId: %s", tx["address"])
		log.Printf("Date: %s", tx["date"])
		log.Printf("Output: %s", tx["output"])
		log.Printf("Value: %s\n", tx["value"])
	}

	// Send transactions to backend
	err = SendTransactionsToBackend(formattedTransactions)
	if err != nil {
		log.Printf("Error sending transactions to backend: %v", err)
	}
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

	backendURL := viper.GetString("relay_backend_url")
	if backendURL == "" {
		log.Println("Using default value, not config.json values.")
		backendURL = "http://localhost:9002" // Default value
	}

	jsonData, err := json.Marshal(transactions)
	if err != nil {
		return fmt.Errorf("error marshaling transactions: %v", err)
	}

	resp, err := http.Post(backendURL+"/transactions", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error sending transactions to backend: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("backend returned non-OK status: %v", resp.Status)
	}

	log.Println("Transactions sent successfully to backend")
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
		"wallet_name": walletName, // Include wallet name in the payload
		"balance":     walletBalance,
	}

	// Marshal data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("error marshaling balance data: %v", err)
	}

	// Send POST request to backend
	resp, err := http.Post(backendURL+"/balance", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error sending balance to backend: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("backend returned non-OK status: %v", resp.Status)
	}

	log.Println("Wallet balance sent successfully to backend")
	return nil
}

func SendReceiveAddressesToBackend(walletName string) error {
	// Retrieve the receive addresses
	receiveAddresses, _, err := walletstatedb.RetrieveAddresses()
	if err != nil {
		return fmt.Errorf("error retrieving receive addresses: %v", err)
	}

	// Prepare the data to send
	var addresses []map[string]string
	for i, addr := range receiveAddresses {
		addresses = append(addresses, map[string]string{
			"index":       fmt.Sprintf("%d", i), // Index of the address
			"address":     addr.EncodeAddress(),
			"wallet_name": walletName, // Include the wallet name
		})
	}

	// Send the addresses to the backend
	backendURL := viper.GetString("relay_backend_url")
	if backendURL == "" {
		log.Println("Using default value, not config.json values.")
		backendURL = "http://localhost:9002" // Default value
	}

	jsonData, err := json.Marshal(addresses)
	if err != nil {
		return fmt.Errorf("error marshaling addresses: %v", err)
	}

	resp, err := http.Post(backendURL+"/addresses", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error sending addresses to backend: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("backend returned non-OK status: %v", resp.Status)
	}

	log.Println("Receive addresses sent successfully to backend")
	return nil
}
