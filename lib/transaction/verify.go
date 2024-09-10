package transaction

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/checksum0/go-electrum/electrum"
)

func verifyTransactionInMempool(txHash chainhash.Hash) (bool, error) {
	stringTxHash := txHash.String()
	url := fmt.Sprintf("https://api.blockcypher.com/v1/btc/main/txs/%s", stringTxHash)

	// Create a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create the request with the context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to get transaction: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to get transaction: status code %d", resp.StatusCode)
		return false, fmt.Errorf("failed to get transaction: status code %d", resp.StatusCode)
	}

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		log.Printf("Failed to decode response: %v", err)
		return false, fmt.Errorf("failed to decode response: %v", err)
	}

	if result["hash"] == txHash.String() {
		fmt.Println("Transaction is in the mempool")
		return true, nil
	} else {
		fmt.Println("Transaction is not in the mempool or already confirmed")
		return false, nil
	}
}

func CreateElectrumClient(config ElectrumConfig) (*electrum.Client, error) {
	ctx := context.Background()
	if config.UseSSL {
		return electrum.NewClientSSL(ctx, config.ServerAddr, nil)
	}
	return electrum.NewClientTCP(ctx, config.ServerAddr)
}

func VerifyTransactionInElectrumMempool(client *electrum.Client, txid string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := client.GetRawTransaction(ctx, txid)
	if err != nil {
		return false, fmt.Errorf("error checking Electrum mempool: %v", err)
	}
	return tx != "", nil
}

func GetAndPrintTransaction(client *electrum.Client, txid string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := client.GetRawTransaction(ctx, txid)
	if err != nil {
		return "", fmt.Errorf("error checking Electrum mempool: %v", err)
	}
	if tx == "" {
		fmt.Printf("Transaction %s not found in the mempool.\n", txid)
		return "", nil
	}
	fmt.Printf("Transaction %s found in the mempool: %v\n", txid, tx)
	return tx, nil
}
