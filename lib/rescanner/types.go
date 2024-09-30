package rescanner

import (
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
)

type RescanConfig struct {
	ChainClient *chain.NeutrinoClient
	ChainParams *chaincfg.Params
	StartBlock  int32
	Wallet      *wallet.Wallet
}
