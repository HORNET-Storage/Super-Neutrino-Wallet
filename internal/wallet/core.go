package wallet

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/api"
	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/logger"
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

const (
	syncInterval      = 10 * time.Minute
	useHTTPS     bool = false
)

func NewWalletServer(wallet *wallet.Wallet, chainParams *chaincfg.Params, chainService *neutrino.ChainService,
	chainClient *chain.NeutrinoClient, neutrinoDB walletdb.DB, privPass []byte,
	name string, httpMode bool) *WalletServer {
	return &WalletServer{
		API: api.NewAPI(wallet, chainParams, chainService, chainClient, neutrinoDB, privPass, name, httpMode),
	}
}

func StartWallet(seedPhrase string, pubPass []byte, privPass []byte, baseDir string, walletName string, birthdate time.Time, httpMode bool) error {

	// Ensure the JWT key is available
	if err := api.EnsureJWTKey(walletName); err != nil {
		log.Printf("Failed to initialize JWT key: %v", err)
	}

	server, err := initializeWalletServer(seedPhrase, pubPass, privPass, baseDir, walletName, birthdate, useHTTPS)
	if err != nil {
		return err
	}
	defer server.Close()

	log.Println("Bitcoin wallet application initialized successfully")
	logger.Info("Bitcoin wallet application initialized successfully")

	if httpMode {
		return server.StartHTTPSServer() // Start the HTTP server
	} else {
		return server.Run() // Start the terminal interface
	}
}

func initializeWalletServer(seedPhrase string, pubPass []byte, privPass []byte, baseDir string, walletName string, birthdate time.Time, httpMode bool) (*WalletServer, error) {
	// Initialize the wallet and necessary components
	w, chainParams, chainService, chainClient, neutrinoDB, err := initializeWallet(seedPhrase, pubPass, privPass, baseDir, walletName, birthdate)
	if err != nil {
		return nil, err
	}

	server := NewWalletServer(w, chainParams, chainService, chainClient, neutrinoDB, privPass, walletName, httpMode)

	return server, nil
}

func (s *WalletServer) Run() error {
	if s.API.HttpMode {
		log.Println("Starting in HTTP server mode")
		return s.StartHTTPSServer() // Start the HTTP server if in HTTP mode
	} else {
		log.Println("Starting in terminal mode")
		return s.serverLoop() // Run the terminal interface
	}
}

func (s *WalletServer) Close() {
	if s.API.Wallet != nil {
		s.API.Wallet.Lock() // Lock the wallet to ensure it's safe to close
	}

	if s.API.ChainClient != nil {
		s.API.ChainClient.Stop()
	}

	if s.API.ChainService != nil {
		s.API.ChainService.Stop()

	}

	if s.API.NeutrinoDB != nil {
		s.API.NeutrinoDB.Close()
	}
}

func initializeWallet(seedPhrase string, pubPass []byte, privPass []byte, baseDir string, walletName string, birthdate time.Time) (*wallet.Wallet, *chaincfg.Params, *neutrino.ChainService, *chain.NeutrinoClient, walletdb.DB, error) {
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

	walletDbName := fmt.Sprintf("%s_wallet.db", walletName)

	walletDBPath := filepath.Join(walletDir, walletDbName)

	dbTimeout := time.Second * 120
	recoveryWindow := uint32(250)
	loader := wallet.NewLoader(chainParams, walletDir, false, dbTimeout, recoveryWindow)
	log.Printf("Wallet loader initialized with timeout: %v", dbTimeout)

	forceNewWallet := false
	log.Printf("Force create new wallet creation: %v", forceNewWallet)

	// TODO: Remove this once all testing it done.
	if forceNewWallet {
		if err := CleanupExistingData(neutrinoDBPath, walletDBPath); err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("error cleaning up existing data: %v", err)
		}
		log.Println("Existing data cleaned up successfully")
	}

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

	if walletExists && !forceNewWallet {
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

		lastScannedHeight := estimateBlockHeight(birthday)

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
	accountKey, err := deriveKeyFromPath(rootKey, accountKeyPath)
	if err != nil {
		log.Fatalf("Error deriving account key: %v", err)
	}

	// Get xpub and zpub
	_, err = GetExtendedPubKey(accountKey, []byte{0x04, 0x88, 0xB2, 0x1E})
	if err != nil {
		log.Fatalf("Error getting xpub: %v", err)
	}

	_, err = GetExtendedPubKey(accountKey, []byte{0x04, 0xB2, 0x47, 0x46})
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

	initialChainServiceSync(chainService)

	chainClient, err := initializeChainClient(chainParams, chainService)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error initializing chain client: %v", err)
	}

	err = chainClient.Start()
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error starting chain client: %v", err)
	}

	w.SynchronizeRPC(chainClient)

	handleAddressGeneration(w, chainClient, needsAddresses, freshWallet)

	PerformRescanAndProcessTransactions(w, chainClient, chainParams, walletName)

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

// StartHTTPSServer starts the HTTPS server, generates the certificate if necessary, and trusts the certificate based on the OS
func (s *WalletServer) StartHTTPSServer() error {
	// Start the background sync process
	go s.startSyncProcess()

	// Wrap your handlers with the CORS middleware
	http.HandleFunc("/transaction", s.API.CORSMiddleware(s.API.JWTMiddleware(s.API.TransactionHandler)))
	http.HandleFunc("/calculate-tx-size", s.API.CORSMiddleware(s.API.JWTMiddleware(s.API.HandleTransactionSizeEstimate)))

	// Route for challenge generation
	http.HandleFunc("/challenge", s.API.CORSMiddleware(s.API.HandleChallengeRequest))

	// Route for verifying challenge and issuing JWT
	http.HandleFunc("/verify", s.API.CORSMiddleware(s.API.VerifyChallenge))

	// Set up the server configuration (common for both HTTP and HTTPS)
	server := &http.Server{
		Addr:         ":9003", // Default HTTP port
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if useHTTPS {
		// Change the address to :443 if using HTTPS
		server.Addr = ":443"

		// Check if cert/key exists, if not, generate them
		if _, err := os.Stat("server.crt"); os.IsNotExist(err) {
			err := generateSelfSignedCert()
			if err != nil {
				log.Fatalf("Failed to generate certificates: %v", err)
			}

			// Trust the certificate based on the OS
			err = trustCertificate("server.crt")
			if err != nil {
				log.Fatalf("Failed to trust the certificate: %v", err)
			}
		}

		// Set TLS configuration (specific to HTTPS)
		server.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS13, // Enforce TLS 1.3 or higher
			CipherSuites: []uint16{
				tls.TLS_AES_128_GCM_SHA256,
				tls.TLS_AES_256_GCM_SHA384,
				tls.TLS_CHACHA20_POLY1305_SHA256,
			},
		}

		log.Println("Starting HTTPS server on :443")
		return server.ListenAndServeTLS("server.crt", "server.key")
	}

	// If not using HTTPS, start the HTTP server
	log.Println("Starting HTTP server on :80")
	return server.ListenAndServe()
}

func (s *WalletServer) startSyncProcess() {
	syncTicker := time.NewTicker(syncInterval)
	defer syncTicker.Stop() // Ensure the ticker is properly stopped when done

	for range syncTicker.C {
		if !transacting {
			log.Println("Starting periodic sync process...")
			engaged = true
			s.syncBlockchain()

			s.API.ChainClient.Notifications()

			s.API.Wallet.SynchronizeRPC(s.API.ChainClient)

			PerformRescanAndProcessTransactions(s.API.Wallet, s.API.ChainClient, s.API.ChainParams, s.API.Name)

			// Update the last scanned block height
			_, lastScannedHeight, err := s.API.ChainClient.GetBestBlock()
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

			engaged = false
			log.Println("Sync process completed.")
		}
	}
}
