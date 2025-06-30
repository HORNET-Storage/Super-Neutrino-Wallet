package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/golang-jwt/jwt/v4"
	"github.com/lightninglabs/neutrino"
	"github.com/spf13/viper"
)

func NewAPI(wallet *wallet.Wallet, chainParams *chaincfg.Params, chainService *neutrino.ChainService,
	chainClient *chain.NeutrinoClient, neutrinoDB walletdb.DB, privPass []byte,
	name string, httpMode bool) *API {
	return &API{
		Wallet:       wallet,
		ChainParams:  chainParams,
		ChainService: chainService,
		ChainClient:  chainClient,
		NeutrinoDB:   neutrinoDB,
		PrivPass:     privPass,
		Name:         name,
		HttpMode:     httpMode,
	}
}

func (s *API) CORSMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		allowedOrigin := viper.GetString("allowed_origin")
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	}
}

func (a *API) JWTMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("Checking Token.")

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			log.Println("Authorization header missing.")
			http.Error(w, "Unauthorized: Authorization header missing", http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			log.Println("Invalid Authorization header format.")
			http.Error(w, "Unauthorized: Invalid token format", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		log.Println("token string: ", tokenString)

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return GetJWTKey(), nil
		})

		if err != nil {
			if validationErr, ok := err.(*jwt.ValidationError); ok {
				if validationErr.Errors == jwt.ValidationErrorExpired {
					log.Println("Token expired.")
					http.Error(w, "Token expired", http.StatusUnauthorized)
					return
				}
			}
			log.Println("Invalid token:", err)
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		if !token.Valid {
			log.Println("Token is not valid.")
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		log.Println("Token is valid.")
		next.ServeHTTP(w, r)
	}
}

// WalletAPIClaims represents claims for wallet API communication
type WalletAPIClaims struct {
	APIKey string `json:"api_key"`
	jwt.RegisteredClaims
}

// generateWalletAPIToken creates a JWT token specifically for wallet API communication
func GenerateWalletAPIToken(apiKey string) (string, error) {
	claims := WalletAPIClaims{
		APIKey: apiKey,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(apiKey))
}

// WalletAPIMiddleware verifies JWT tokens created with the shared API key
func (a *API) WalletAPIMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("Checking Wallet API Token")

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			log.Println("Authorization header missing")
			http.Error(w, "Unauthorized: Authorization header missing", http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			log.Println("Invalid Authorization header format")
			http.Error(w, "Unauthorized: Invalid token format", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// Get the API key from request headers
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			log.Println("API Key missing")
			http.Error(w, "Unauthorized: API Key missing", http.StatusUnauthorized)
			return
		}

		// Verify that API key matches configuration
		expectedAPIKey := viper.GetString("wallet_api_key")
		if apiKey != expectedAPIKey {
			log.Println("Invalid API Key")
			http.Error(w, "Unauthorized: Invalid API Key", http.StatusUnauthorized)
			return
		}

		// Parse and validate the token using the API key as the secret
		claims := &WalletAPIClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(apiKey), nil
		})

		if err != nil {
			log.Printf("Token validation error: %v", err)
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		if !token.Valid {
			log.Println("Token is not valid")
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		// Verify that the API key in claims matches the header
		if claims.APIKey != apiKey {
			log.Println("Token API key mismatch")
			http.Error(w, "Unauthorized: Token mismatch", http.StatusUnauthorized)
			return
		}

		log.Println("Wallet API Token is valid")
		next.ServeHTTP(w, r)
	}
}

type HealthStatus struct {
	Status       string `json:"status"`
	Timestamp    string `json:"timestamp"`
	WalletLocked bool   `json:"wallet_locked"`
	ChainSynced  bool   `json:"chain_synced"`
	PeerCount    int32  `json:"peer_count"`
}

type HealthCheckRequest struct {
	RequestID string `json:"request_id,omitempty"`
}

func (a *API) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var req HealthCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	health := HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if a.Wallet != nil {
		health.WalletLocked = a.Wallet.Locked()
	}

	if a.ChainService != nil {
		health.ChainSynced = a.ChainService.IsCurrent()
		peers, _, _ := a.ChainService.ConnectedPeers()
		health.PeerCount = int32(len(peers))
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(health)
}

func (a *API) HandlePanelHealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	health := HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if a.Wallet != nil {
		health.WalletLocked = a.Wallet.Locked()
	}

	if a.ChainService != nil {
		health.ChainSynced = a.ChainService.IsCurrent()
		peers, _, _ := a.ChainService.ConnectedPeers()
		health.PeerCount = int32(len(peers))
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(health)
}
