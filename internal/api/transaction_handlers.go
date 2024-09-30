package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/Maphikza/btc-wallet-btcsuite.git/lib/transaction"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

func (s *API) PerformHttpTransaction(req TransactionRequest) (chainhash.Hash, string, string) {
	enableRBF := req.EnableRBF
	var txid chainhash.Hash
	var status, message string

	switch req.Choice {
	case 1:
		// New transaction
		txid, verified, err := transaction.HttpCheckBalanceAndCreateTransaction(s.Wallet, s.ChainClient.CS, enableRBF, req.SpendAmount, req.RecipientAddress, s.PrivPass, req.PriorityRate)
		if err != nil {
			message = fmt.Sprintf("Error creating or broadcasting transaction: %v", err)
			status = "failed"
		} else if verified {
			message = "Transaction successfully broadcasted and verified in the mempool"
			status = "success"
		} else {
			message = "Transaction broadcasted. Please check the mempool in a few seconds to see if it is confirmed."
			status = "pending"
		}

		return txid, status, message

	case 2:
		// RBF (Replace-By-Fee) transaction
		var mempoolSpaceConfig = transaction.ElectrumConfig{
			ServerAddr: "electrum.blockstream.info:50002",
			UseSSL:     true,
		}
		client, err := transaction.CreateElectrumClient(mempoolSpaceConfig)
		if err != nil {
			return chainhash.Hash{}, "failed", fmt.Sprintf("Failed to create Electrum client: %v", err)
		}
		txid, verified, err := transaction.ReplaceTransactionWithHigherFee(s.Wallet, s.ChainClient.CS, req.OriginalTxID, req.NewFeeRate, client, s.PrivPass)
		if err != nil {
			message = fmt.Sprintf("Error performing RBF transaction: %v", err)
			status = "failed"
		} else if verified {
			message = "RBF transaction successfully broadcasted and verified in the mempool"
			status = "success"
		} else {
			message = "RBF transaction broadcasted. Please check the mempool in a few seconds."
			status = "pending"
		}
		return txid, status, message

	default:
		message = "Invalid transaction choice"
		status = "failed"
	}

	return txid, status, message
}

func (s *API) TransactionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var req TransactionRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	txid, status, message := s.performHttpTransaction(req)

	resp := TransactionResponse{
		TxID:    txid.String(),
		Status:  status,
		Message: message,
	}

	// Convert the response struct to a JSON string for logging
	respJson, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
	} else {
		log.Println("Response: ", string(respJson))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *API) HandleTransactionSizeEstimate(w http.ResponseWriter, r *http.Request) {
	// Parse the request body for parameters (spend amount, recipient address, etc.)
	var req TransactionRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Call the transaction size estimator function
	txSize, err := transaction.HttpCalculateTransactionSize(s.Wallet, req.SpendAmount, req.RecipientAddress, req.PriorityRate)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to estimate transaction size: %v", err), http.StatusInternalServerError)
		return
	}

	// Send back the transaction size
	resp := map[string]int{
		"txSize": txSize,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *API) performHttpTransaction(req TransactionRequest) (chainhash.Hash, string, string) {
	enableRBF := req.EnableRBF
	var txid chainhash.Hash
	var status, message string

	switch req.Choice {
	case 1:
		// New transaction
		txid, verified, err := transaction.HttpCheckBalanceAndCreateTransaction(s.Wallet, s.ChainClient.CS, enableRBF, req.SpendAmount, req.RecipientAddress, s.PrivPass, req.PriorityRate)
		if err != nil {
			message = fmt.Sprintf("Error creating or broadcasting transaction: %v", err)
			status = "failed"
		} else if verified {
			message = "Transaction successfully broadcasted and verified in the mempool"
			status = "success"
		} else {
			message = "Transaction broadcasted. Please check the mempool in a few seconds to see if it is confirmed."
			status = "pending"
		}

		return txid, status, message

	case 2:
		// RBF (Replace-By-Fee) transaction
		var mempoolSpaceConfig = transaction.ElectrumConfig{
			ServerAddr: "electrum.blockstream.info:50002",
			UseSSL:     true,
		}
		client, err := transaction.CreateElectrumClient(mempoolSpaceConfig)
		if err != nil {
			return chainhash.Hash{}, "failed", fmt.Sprintf("Failed to create Electrum client: %v", err)
		}
		txid, verified, err := transaction.ReplaceTransactionWithHigherFee(s.Wallet, s.ChainClient.CS, req.OriginalTxID, req.NewFeeRate, client, s.PrivPass)
		if err != nil {
			message = fmt.Sprintf("Error performing RBF transaction: %v", err)
			status = "failed"
		} else if verified {
			message = "RBF transaction successfully broadcasted and verified in the mempool"
			status = "success"
		} else {
			message = "RBF transaction broadcasted. Please check the mempool in a few seconds."
			status = "pending"
		}
		return txid, status, message

	default:
		message = "Invalid transaction choice"
		status = "failed"
	}

	return txid, status, message
}
