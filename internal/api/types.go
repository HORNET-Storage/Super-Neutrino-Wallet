package api

import (
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightninglabs/neutrino"
)

type API struct {
	Wallet       *wallet.Wallet
	ChainParams  *chaincfg.Params
	ChainService *neutrino.ChainService
	ChainClient  *chain.NeutrinoClient
	NeutrinoDB   walletdb.DB
	PrivPass     []byte
	Name         string
	HttpMode     bool
}

type TransactionRequest struct {
	Choice           int    `json:"choice"`
	RecipientAddress string `json:"recipient_address"`
	SpendAmount      int64  `json:"spend_amount"`
	PriorityRate     int    `json:"priority_rate"`
	FilePath         string `json:"file_path,omitempty"`
	OriginalTxID     string `json:"original_tx_id,omitempty"`
	NewFeeRate       int64  `json:"new_fee_rate,omitempty"`
	EnableRBF        bool   `json:"enable_rbf"` // New field for enabling RBF
}

type TransactionResponse struct {
	TxID    string `json:"txid"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type contextKey string
