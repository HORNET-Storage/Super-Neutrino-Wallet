package chain

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/lightninglabs/neutrino"
)

func InitializeChainClient(chainParams *chaincfg.Params, chainService *neutrino.ChainService) (*chain.NeutrinoClient, error) {
	chainClient := chain.NewNeutrinoClient(chainParams, chainService)
	err := chainClient.Start()
	if err != nil {
		return nil, fmt.Errorf("error starting chain client: %v", err)
	}
	return chainClient, nil
}
