package api

import (
	"log"
	"net/http"
	"strings"

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
