package transaction

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/neutrino"
)

func BroadcastTransactionMultiAPI(tx *wire.MsgTx) error {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return fmt.Errorf("failed to serialize transaction: %v", err)
	}
	txHex := hex.EncodeToString(buf.Bytes())

	// Try mempool.space API
	err := broadcastToMempoolSpace(txHex)
	if err == nil {
		return nil
	}
	log.Printf("mempool.space broadcast failed: %v. Trying BlockCypher...", err)

	// Try BlockCypher API
	err = broadcastToBlockCypher(txHex)
	if err == nil {
		return nil
	}
	log.Printf("BlockCypher broadcast failed: %v. Trying Blockstream...", err)

	// Try Blockstream API
	err = broadcastToBlockstream(txHex)
	if err == nil {
		return nil
	}
	log.Printf("Blockstream broadcast failed: %v", err)

	return fmt.Errorf("all API broadcasts failed")
}

func broadcastToMempoolSpace(txHex string) error {
	url := "https://mempool.space/api/tx"
	return broadcastToAPI(url, txHex, "text/plain")
}

func broadcastToBlockCypher(txHex string) error {
	url := "https://api.blockcypher.com/v1/btc/main/txs/push"
	jsonData := map[string]string{"tx": txHex}
	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %v", err)
	}
	return broadcastToAPI(url, string(jsonBytes), "application/json")
}

func broadcastToBlockstream(txHex string) error {
	url := "https://blockstream.info/api/tx"
	return broadcastToAPI(url, txHex, "text/plain")
}

func broadcastToAPI(url, data, contentType string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, contentType, bytes.NewBufferString(data))
	if err != nil {
		return fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode == http.StatusOK {
		log.Printf("Transaction broadcast successfully via %s. Response: %s", url, string(body))
		return nil
	}

	return fmt.Errorf("API returned non-200 status code: %d, Body: %s", resp.StatusCode, string(body))
}

func broadcastAndVerifyTransaction(tx *wire.MsgTx, service *neutrino.ChainService) (chainhash.Hash, bool, error) {
	// Start with multi-API broadcast
	err := BroadcastTransactionMultiAPI(tx)
	if err == nil {
		log.Printf("Transaction broadcast successfully via API. TxID: %s", tx.TxHash().String())
		return tx.TxHash(), true, nil
	}

	log.Printf("API broadcast failed: %v. Trying neutrino ChainService...", err)

	// Fallback to neutrino ChainService if API broadcast fails
	err = service.SendTransaction(tx)
	if err == nil {
		log.Printf("Transaction broadcast via neutrino ChainService. Verifying mempool...")

		time.Sleep(5 * time.Second)

		// After sending the transaction, immediately verify if it is in the mempool
		inMempool, err := verifyTransactionInMempool(tx.TxHash())
		if err != nil {
			log.Printf("Mempool verification failed after neutrino broadcast: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("neutrino broadcast succeeded but mempool check failed: %v", err)
		}

		if inMempool {
			log.Printf("Transaction successfully broadcast and found in mempool. TxID: %s", tx.TxHash().String())
			return tx.TxHash(), true, nil
		}

		// If not found in mempool, treat as failure despite the successful broadcast call
		log.Printf("Neutrino broadcast succeeded but transaction not found in mempool. TxID: %s", tx.TxHash().String())
		return chainhash.Hash{}, false, fmt.Errorf("neutrino broadcast succeeded but transaction not found in mempool")
	}

	log.Printf("Neutrino ChainService broadcast failed: %v", err)

	// If we reach this point, all broadcast attempts have failed
	return chainhash.Hash{}, false, fmt.Errorf("all broadcast attempts failed: %v", err)
}
