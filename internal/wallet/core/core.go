package core

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/addresses"
	snWalletChain "github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/chain"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/formatter"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/utils"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightninglabs/neutrino"
	"github.com/spf13/viper"
	"github.com/tyler-smith/go-bip39"
)

func InitializeWallet(seedPhrase string, pubPass []byte, privPass []byte, baseDir string, walletName string, birthdate time.Time) (*wallet.Wallet, *chaincfg.Params, *neutrino.ChainService, *chain.NeutrinoClient, walletdb.DB, error) {
	// Initialize the wallet and necessary components
	// Initialize the database
	walletGravitonDbName := fmt.Sprintf("%s_wallet_graviton.db", walletName)
	dbPath := filepath.Join(baseDir, walletGravitonDbName)
	err := walletstatedb.InitDB(dbPath)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error initializing database: %v", err)
	}

	log.Println("Generating BIP39 seed from seed phrase")
	seed, err := bip39.NewSeedWithErrorChecking(seedPhrase, "")
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error generating seed: %v", err)
	}

	// Generating root key from seed
	rootKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error generating root key: %v", err)
	}

	chainParams := &chaincfg.MainNetParams

	log.Printf("Using base directory: %s", baseDir)

	neutrinoDBPath := filepath.Join(baseDir, "neutrino_db")
	if err := os.MkdirAll(neutrinoDBPath, os.ModePerm); err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error creating Neutrino DB directory: %v", err)
	}

	walletDirName := fmt.Sprintf("%s_wallet", walletName)

	walletDir := filepath.Join(neutrinoDBPath, walletDirName)
	if err := os.MkdirAll(walletDir, os.ModePerm); err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error creating wallet directory: %v", err)
	}

	dbTimeout := time.Second * 120
	recoveryWindow := uint32(250)
	loader := wallet.NewLoader(chainParams, walletDir, false, dbTimeout, recoveryWindow)
	log.Printf("Wallet loader initialized with timeout: %v", dbTimeout)

	walletExists, err := loader.WalletExists()
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error checking wallet existence: %v", err)
	}

	if walletExists {
		log.Println("Wallet already exists, skipping wallet creation")
	}

	var w *wallet.Wallet
	var receiveAddresses, changeAddresses []btcutil.Address
	var needsAddresses bool
	var freshWallet bool

	if walletExists {
		log.Println("Attempting to open existing wallet")
		openStart := time.Now()
		w, err = loader.OpenExistingWallet(pubPass, false)
		openDuration := time.Since(openStart)
		if err != nil {
			log.Printf("Error opening wallet after %v: %v", openDuration, err)
		}
		log.Printf("Existing wallet opened successfully after %v", openDuration)

		log.Println("Checking database for existing addresses")
		receiveAddresses, changeAddresses, err = walletstatedb.RetrieveAddresses()
		if err != nil {
			log.Printf("Error retrieving existing addresses: %v", err)
		} else {
			log.Printf("Found %d receive addresses and %d change addresses in the database", len(receiveAddresses), len(changeAddresses))
			if len(receiveAddresses) == 0 && len(changeAddresses) == 0 {
				log.Println("No addresses found in the database. Will generate initial addresses after sync.")
				needsAddresses = true
			} else {
				log.Println("Addresses found in the database. Will not generate initial addresses after sync.")
			}
		}
	} else {
		log.Println("Creating new wallet")
		createStart := time.Now()
		birthday := birthdate

		lastScannedHeight := utils.EstimateBlockHeight(birthday)

		log.Println("Estimate block height from birthday: ", lastScannedHeight)

		err = walletstatedb.UpdateLastScannedBlockHeight(lastScannedHeight)
		if err != nil {
			log.Printf("Error updating last scanned block height: %v", err)
		} else {
			log.Printf("Updated last scanned block height to %d", lastScannedHeight)
		}
		w, err = loader.CreateNewWalletExtendedKey(pubPass, privPass, rootKey, birthday)
		createDuration := time.Since(createStart)
		if err != nil {
			log.Printf("Error creating wallet after %v: %v", createDuration, err)
			log.Fatalf("Error creating wallet: %v", err)
		}
		log.Printf("New wallet created successfully after %v", createDuration)
		log.Println("New wallet created. Will generate initial addresses after sync.")

		if isBirthdayToday(birthday) {
			freshWallet = true
		} else {
			freshWallet = false
			needsAddresses = true

		}
	}

	// Derive the account key from m/84'/0'/0'
	accountKeyPath := "84'/0'/0'"
	accountKey, err := utils.DeriveKeyFromPath(rootKey, accountKeyPath)
	if err != nil {
		log.Fatalf("Error deriving account key: %v", err)
	}

	// Get xpub and zpub
	_, err = utils.GetExtendedPubKey(accountKey, []byte{0x04, 0x88, 0xB2, 0x1E})
	if err != nil {
		log.Fatalf("Error getting xpub: %v", err)
	}

	_, err = utils.GetExtendedPubKey(accountKey, []byte{0x04, 0xB2, 0x47, 0x46})
	if err != nil {
		log.Fatalf("Error getting zpub: %v", err)
	}

	log.Println("Initializing Neutrino chain service")
	db, err := walletdb.Create("bdb", filepath.Join(neutrinoDBPath, "neutrino.db"), true, time.Second*60)
	if err != nil {
		log.Fatalf("Error creating Neutrino database: %v", err)
	}
	addPeers := viper.GetStringSlice("add_peers")
	if len(addPeers) == 0 {
		// Fallback to default peers if none are configured
		addPeers = []string{
			"seed.bitcoin.sipa.be:8333",
			"dnsseed.bluematt.me:8333",
		}
		log.Println("Using default AddPeers as none were configured")
	} else {
		log.Println("Using AddPeers from configuration")
	}

	cfg := neutrino.Config{
		DataDir:         neutrinoDBPath,
		Database:        db,
		ChainParams:     *chainParams,
		AddPeers:        addPeers,
		PersistToDisk:   true,
		FilterCacheSize: neutrino.DefaultFilterCacheSize,
		BlockCacheSize:  neutrino.DefaultBlockCacheSize,
	}

	chainService, err := neutrino.NewChainService(cfg)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error creating chain service: %v", err)
	}

	log.Println("Starting chain service")
	if err := chainService.Start(); err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error starting chain service: %v", err)
	}

	InitialChainServiceSync(chainService)

	chainClient, err := snWalletChain.InitializeChainClient(chainParams, chainService)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error initializing chain client: %v", err)
	}

	err = chainClient.Start()
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error starting chain client: %v", err)
	}

	log.Println("Wallet is initiated? ", w.Locked())

	// Ensure that the wallet and chain client are properly initialized before calling SynchronizeRPC
	if w == nil {
		log.Println("Wallet is nil, cannot synchronize RPC")
		return nil, nil, nil, nil, nil, fmt.Errorf("wallet is not properly initialized")
	}

	if chainClient == nil {
		log.Println("Chain client is nil, cannot synchronize RPC")
		return nil, nil, nil, nil, nil, fmt.Errorf("chain client is not properly initialized")
	}

	log.Println("Wallet is initiated? ", w.Locked())

	// Proceed with synchronization only if the wallet is locked
	if w.Locked() {
		log.Println("Synchronizing RPC with the chain client")
		w.SynchronizeRPC(chainClient)
	} else {
		log.Println("Wallet is not locked, skipping SynchronizeRPC call")
	}

	addresses.HandleAddressGeneration(w, chainClient, needsAddresses, freshWallet)

	formatter.PerformRescanAndProcessTransactions(w, chainClient, chainParams, walletName)

	_, lastScannedHeight, err := chainClient.GetBestBlock()
	if err != nil {
		log.Printf("Error getting last scanned block height: %v", err)
	} else {
		err = walletstatedb.UpdateLastScannedBlockHeight(lastScannedHeight)
		if err != nil {
			log.Printf("Error updating last scanned block height: %v", err)
		} else {
			log.Printf("Updated last scanned block height to %d", lastScannedHeight)
		}
	}

	return w, chainParams, chainService, chainClient, db, nil
}

func isBirthdayToday(birthday time.Time) bool {
	today := time.Now()
	return birthday.Month() == today.Month() &&
		birthday.Day() == today.Day() &&
		birthday.Year() == today.Year()
}

func InitialChainServiceSync(chainService *neutrino.ChainService) {
	log.Println("Starting initial syncing process...")

	for i := 0; i < 120; i++ {
		time.Sleep(10 * time.Second)
		bestBlock, err := chainService.BestBlock()
		if err != nil {
			log.Printf("Error getting best block: %v", err)
			continue
		}
		log.Printf("Current block height: %d", bestBlock.Height)

		currentHash, err := chainService.GetBlockHash(int64(bestBlock.Height))
		if err != nil {
			log.Printf("Error getting current block hash: %v", err)
		} else {
			log.Printf("Current block hash: %s", currentHash.String())
		}

		peers := chainService.Peers()
		log.Printf("Connected peers: %d", len(peers))

		if chainService.IsCurrent() {
			log.Println("Chain is synced!")
			break
		}
	}
}
