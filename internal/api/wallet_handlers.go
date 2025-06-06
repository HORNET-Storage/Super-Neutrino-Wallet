package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/addresses"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/formatter"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/golang-jwt/jwt/v4"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"
)

// AddressGenerationRequest represents the incoming request for new addresses
type AddressGenerationRequest struct {
	Count int `json:"count"`
}

func (s *API) HandleAddressGeneration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the request
	var req AddressGenerationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Error decoding request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Count <= 0 || req.Count > 1000 {
		http.Error(w, "Invalid count: must be between 1 and 1000", http.StatusBadRequest)
		return
	}

	log.Printf("Generating %d new addresses for wallet: %s", req.Count, s.Name)

	// Generate addresses using the existing functionality
	_, _, err := addresses.GenerateAndSaveAddresses(s.Wallet, req.Count)
	if err != nil {
		log.Printf("Error generating addresses: %v", err)
		http.Error(w, "Failed to generate addresses", http.StatusInternalServerError)
		return
	}

	err = formatter.SendReceiveAddressesToBackend(s.Name)
	if err != nil {
		log.Printf("Failed to send address to backend: %s", err)
		http.Error(w, "Failed to send addresses to backend", http.StatusInternalServerError)
		return
	}

	// Send success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Successfully generated %d addresses", req.Count),
	})
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

	// Save the challenge in the SQLite database
	newChallenge := walletstatedb.Challenge{
		Challenge: challenge,
		Hash:      hash,
		Status:    "unused",
		Npub:      pubKey,
		CreatedAt: time.Now(),
	}

	if err := walletstatedb.SaveChallenge(newChallenge); err != nil {
		http.Error(w, "Failed to save challenge", http.StatusInternalServerError)
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

	// Retrieve the challenge from the database
	challengeHash := sha256.Sum256([]byte(verifyPayload.Challenge))
	hashString := hex.EncodeToString(challengeHash[:])

	challenge, err := walletstatedb.GetChallenge(hashString)

	if err != nil || challenge.Status != "unused" {
		http.Error(w, "Invalid or expired challenge", http.StatusUnauthorized)
		return
	}

	// Check if the challenge has expired (older than 2 minutes)
	if time.Since(challenge.CreatedAt) > 2*time.Minute {
		walletstatedb.MarkChallengeAsUsed(challenge.Hash)
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
	if err := walletstatedb.MarkChallengeAsUsed(challenge.Hash); err != nil {
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
