// File: internal/api/middleware.go

package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/logger"
	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/viper"
)

var jwtKey []byte

// JWT Claims
type Claims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// LoggingMiddleware logs information about each request
func LoggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		next.ServeHTTP(w, r)

		logger.Info("Request processed",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	}
}

// JSONContentTypeMiddleware ensures that requests have the correct content type
func JSONContentTypeMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			contentType := r.Header.Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
		}
		next.ServeHTTP(w, r)
	}
}

// AuthMiddleware checks for a valid JWT token
// Note: This is a placeholder and should be implemented with proper JWT validation
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header is required", http.StatusUnauthorized)
			return
		}

		// TODO: Implement proper JWT validation here
		// For now, we'll just check if the header starts with "Bearer "
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
			return
		}

		// If validation passes, call the next handler
		next.ServeHTTP(w, r)
	}
}

// ErrorMiddleware wraps the handler and catches any panics, returning them as 500 errors
func ErrorMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("Panic occurred", "error", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	}
}

// RequestIDMiddleware adds a unique request ID to each request
func RequestIDMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID() // Implement this function to generate a unique ID
		}

		// Use the custom key type here
		ctx := context.WithValue(r.Context(), contextKey("requestID"), requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// generateRequestID generates a unique request ID
// This is a simple implementation and should be replaced with a more robust solution in production
func generateRequestID() string {
	return time.Now().Format("20060102150405") + "-" + strings.ReplaceAll(time.Now().String(), " ", "-")
}

// ApplyMiddleware applies a list of middleware to a handler
func ApplyMiddleware(h http.HandlerFunc, middleware ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	for _, m := range middleware {
		h = m(h)
	}
	return h
}

func GenerateJWTKey() ([]byte, error) {
	key := make([]byte, 32) // 256 bits
	_, err := rand.Read(key)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT key: %v", err)
	}
	return key, nil
}

func SaveJWTKey(key []byte, walletName string) error {
	encodedKey := base64.StdEncoding.EncodeToString(key)
	keyPath := filepath.Join(viper.GetString("jwt_keys_dir"), walletName, "jwt_key")

	log.Printf("Attempting to save JWT key to %s", keyPath)

	// Write the key to the file
	err := os.WriteFile(keyPath, []byte(encodedKey), 0600)
	if err != nil {
		log.Printf("Error saving JWT key: %v", err)
		return fmt.Errorf("failed to save JWT key: %v", err)
	}

	log.Printf("JWT key saved successfully at %s", keyPath)
	return nil
}

func LoadJWTKey(walletName string) ([]byte, error) {
	keyPath := filepath.Join(viper.GetString("jwt_keys_dir"), walletName, "jwt_key")
	log.Printf("Attempting to load JWT key from %s", keyPath)

	encodedKey, err := os.ReadFile(keyPath)
	if err != nil {
		log.Printf("Error reading JWT key file: %v", err)
		return nil, fmt.Errorf("failed to read JWT key: %v", err)
	}

	key, err := base64.StdEncoding.DecodeString(string(encodedKey))
	if err != nil {
		log.Printf("Error decoding JWT key: %v", err)
		return nil, fmt.Errorf("failed to decode JWT key: %v", err)
	}

	log.Printf("JWT key loaded successfully from %s", keyPath)
	return key, nil
}

func InitJWTKey(walletName string) error {
	log.Printf("Initializing JWT key for wallet: %s", walletName)
	key, err := LoadJWTKey(walletName)

	if err != nil {
		if pathErr, ok := err.(*os.PathError); ok && pathErr.Op == "open" {
			log.Printf("JWT key file not found for wallet: %s", walletName)
			// Return the os.ErrNotExist to indicate the file is missing
			return os.ErrNotExist
		}
		log.Printf("Error loading JWT key: %v", err)
		return err
	}

	jwtKey = key
	log.Printf("JWT key initialized successfully for wallet: %s", walletName)
	return nil
}

func GetJWTKey() []byte {
	return jwtKey
}

func EnsureJWTKey(walletName string) error {
	// Construct the wallet directory path
	walletDir := filepath.Join(viper.GetString("jwt_keys_dir"), walletName)
	log.Printf("Ensuring directory exists for wallet: %s", walletDir)

	// Ensure the wallet directory exists
	if _, dirErr := os.Stat(walletDir); os.IsNotExist(dirErr) {
		// Create the wallet directory if it doesn't exist
		log.Printf("Creating directory: %s", walletDir)
		dirCreateErr := os.MkdirAll(walletDir, 0700)
		if dirCreateErr != nil {
			return fmt.Errorf("failed to create directory for JWT key: %v", dirCreateErr)
		}
		log.Printf("Directory %s created successfully", walletDir)
	}

	// Generate a new JWT key every time
	log.Printf("Generating a new JWT key for wallet: %s", walletName)
	newKey, genErr := GenerateJWTKey()
	if genErr != nil {
		return fmt.Errorf("failed to generate new JWT key: %v", genErr)
	}

	log.Println("New Key: ", hex.EncodeToString(newKey))

	// Save the newly generated key
	log.Printf("Saving new JWT key to file for wallet: %s", walletName)
	saveErr := SaveJWTKey(newKey, walletName)
	if saveErr != nil {
		return fmt.Errorf("failed to save new JWT key: %v", saveErr)
	}

	// Initialize the JWT key in memory
	jwtKey = newKey
	log.Printf("JWT key successfully initialized and saved for wallet: %s", walletName)

	return nil
}
