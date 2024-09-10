package api

import (
	"fmt"

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
