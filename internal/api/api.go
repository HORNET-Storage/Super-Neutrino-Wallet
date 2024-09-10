package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/lib/transaction"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/deroproject/graviton"
	"github.com/golang-jwt/jwt/v4"
	"github.com/lightninglabs/neutrino"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"
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

func (s *API) HandleChallengeRequest(w http.ResponseWriter, _ *http.Request) {
	log.Println("Challenge requested...")
	// User's public key from Viper config (as this wallet has one primary user)
	pubKey := viper.GetString("user_pubkey")
	if pubKey == "" {
		http.Error(w, "Primary user public key not configured", http.StatusInternalServerError)
		return
	}

	// Generate a new challenge
	challenge, hash, err := generateChallenge()
	if err != nil {
		http.Error(w, "Failed to generate challenge", http.StatusInternalServerError)
		return
	}

	// Save the challenge in the Graviton database
	ss, err := walletstatedb.Store.LoadSnapshot(0)
	if err != nil {
		http.Error(w, "Failed to load snapshot", http.StatusInternalServerError)
		return
	}

	challengeTree, err := ss.GetTree(walletstatedb.ChallengeTreeName)
	if err != nil {
		http.Error(w, "Failed to get challenge tree", http.StatusInternalServerError)
		return
	}

	newChallenge := walletstatedb.Challenge{
		Challenge: challenge,
		Hash:      hash,
		Status:    "unused",
		Npub:      pubKey,
		CreatedAt: time.Now(),
	}

	if err := walletstatedb.SaveChallenge(challengeTree, newChallenge); err != nil {
		http.Error(w, "Failed to save challenge", http.StatusInternalServerError)
		return
	}

	// Commit the tree
	if _, err := graviton.Commit(challengeTree); err != nil {
		http.Error(w, "Failed to commit challenge", http.StatusInternalServerError)
		return
	}

	// Return the challenge as part of a Nostr event (for frontend)
	event := &nostr.Event{
		PubKey:    pubKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      1,
		Tags:      nostr.Tags{},
		Content:   challenge,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(event); err != nil {
		http.Error(w, "Failed to encode event", http.StatusInternalServerError)
	}
}

func generateChallenge() (string, string, error) {
	timestamp := time.Now().Format(time.RFC3339Nano)
	letters := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	challenge := make([]byte, 12)
	_, err := rand.Read(challenge)
	if err != nil {
		return "", "", err
	}
	for i := range challenge {
		challenge[i] = letters[challenge[i]%byte(len(letters))]
	}
	fullChallenge := fmt.Sprintf("%s-%s", string(challenge), timestamp)
	hash := sha256.Sum256([]byte(fullChallenge))
	return fullChallenge, hex.EncodeToString(hash[:]), nil
}

func (s *API) VerifyChallenge(w http.ResponseWriter, r *http.Request) {
	log.Println("verifying challenge")
	var verifyPayload struct {
		Challenge   string      `json:"challenge"`
		Signature   string      `json:"signature"`
		MessageHash string      `json:"messageHash"`
		Event       nostr.Event `json:"event"`
	}

	if err := json.NewDecoder(r.Body).Decode(&verifyPayload); err != nil {
		http.Error(w, "Cannot parse JSON", http.StatusBadRequest)
		return
	}

	// Load Graviton snapshot and tree
	ss, err := walletstatedb.Store.LoadSnapshot(0)
	if err != nil {
		http.Error(w, "Failed to load snapshot", http.StatusInternalServerError)
		return
	}

	challengeTree, err := ss.GetTree(walletstatedb.ChallengeTreeName)
	if err != nil {
		http.Error(w, "Failed to get challenge tree", http.StatusInternalServerError)
		return
	}

	// Retrieve the challenge from the database
	challengeHash := sha256.Sum256([]byte(verifyPayload.Challenge))
	hashString := hex.EncodeToString(challengeHash[:])
	challenge, err := walletstatedb.GetChallenge(challengeTree, hashString)
	if err != nil || challenge.Status != "unused" {
		http.Error(w, "Invalid or expired challenge", http.StatusUnauthorized)
		return
	}

	// Check if the challenge has expired (older than 2 minutes)
	if time.Since(challenge.CreatedAt) > 2*time.Minute {
		walletstatedb.MarkChallengeAsUsed(challengeTree, challenge.Hash)
		http.Error(w, "Challenge expired", http.StatusUnauthorized)
		return
	}

	// Verify the signature and pubkey
	if verifyPayload.Event.PubKey != challenge.Npub {
		http.Error(w, "Public key mismatch", http.StatusUnauthorized)
		return
	}

	if !verifyEvent(&verifyPayload.Event) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	// Mark the challenge as used
	if err := walletstatedb.MarkChallengeAsUsed(challengeTree, challenge.Hash); err != nil {
		http.Error(w, "Failed to mark challenge as used", http.StatusInternalServerError)
		return
	}

	// Generate JWT token for future use
	tokenString, err := GenerateJWT(challenge.Npub)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]string{"token": tokenString}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode token", http.StatusInternalServerError)
	}
}

func verifyEvent(event *nostr.Event) bool {
	serialized := serializeEventForID(event)
	log.Println("The Event ID is:", event.ID)
	match, hash := HashAndCompare(serialized, event.ID)
	if match {
		fmt.Println("Hash matches ID:", event.ID)
	} else {
		fmt.Println("Hash does not match ID")
	}
	signatureBytes, _ := hex.DecodeString(event.Sig)
	cleanSignature, _ := schnorr.ParseSignature(signatureBytes)
	publicSignatureBytes, _ := hex.DecodeString(event.PubKey)

	cleanPublicKey, _ := schnorr.ParsePubKey(publicSignatureBytes)

	isValid := cleanSignature.Verify(hash[:], cleanPublicKey)

	if isValid {
		fmt.Println("Signature is valid from my implementation")
	} else {
		fmt.Println("Signature is invalid from my implementation")
	}

	log.Println("Event tags: ", event.Tags)

	isValid, err := event.CheckSignature()
	if err != nil {
		log.Println("Error checking signature:", err)
		return false
	}
	if isValid {
		fmt.Println("Signature is valid")
	} else {
		fmt.Println("Signature is invalid")
	}

	if isValid && match {
		return true
	} else {
		return false
	}
}

func serializeEventForID(event *nostr.Event) []byte {
	// Assuming the Tags and other fields are already correctly filled except ID and Sig
	serialized, err := json.Marshal([]interface{}{
		0,
		event.PubKey,
		event.CreatedAt,
		event.Kind,
		event.Tags,
		event.Content,
	})
	if err != nil {
		panic(err) // Handle error properly in real code
	}

	return serialized
}

func HashAndCompare(data []byte, hash string) (bool, []byte) {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]) == hash, h[:]
}

// Function to generate JWT token (called upon user login or request)
func GenerateJWT(userID string) (string, error) {
	expirationTime := time.Now().Add(15 * time.Minute)
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	// Retrieve the JWT key using GetJWTKey()
	signingKey := GetJWTKey()
	if signingKey == nil {
		return "", fmt.Errorf("JWT signing key not available")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(signingKey)
	if err != nil {
		return "", err
	}

	log.Println("Generated JWT token:", tokenString)

	return tokenString, nil
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
