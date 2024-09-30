package wallet

import (
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/api"
)

type WalletServer struct {
	API *api.API
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

type Config struct {
	BackendURL string   `mapstructure:"backend_url"`
	AddPeers   []string `mapstructure:"add_peers"`
}
