package operations

import (
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/api"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/logger"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/core"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/utils"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightninglabs/neutrino"
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

func initializeWalletServer(seedPhrase string, pubPass []byte, privPass []byte, baseDir string, walletName string, birthdate time.Time, httpMode bool) (*WalletServer, error) {
	// Initialize the wallet and necessary components
	w, chainParams, chainService, chainClient, neutrinoDB, err := core.InitializeWallet(seedPhrase, pubPass, privPass, baseDir, walletName, birthdate)
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

// StartHTTPSServer starts the HTTPS server, generates the certificate if necessary, and trusts the certificate based on the OS
func (s *WalletServer) StartHTTPSServer() error {
	// Start the background sync process
	go s.StartSyncProcess()

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
			err := utils.GenerateSelfSignedCert()
			if err != nil {
				log.Fatalf("Failed to generate certificates: %v", err)
			}

			// Trust the certificate based on the OS
			err = utils.TrustCertificate("server.crt")
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
