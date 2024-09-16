package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// LoadConfig loads the configuration and sets default values for development/production
func LoadConfig() error {
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(".") // Path to look for the config file in

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; create a default one
			return createDefaultConfig()
		}
		return fmt.Errorf("error reading config file: %w", err)
	}

	// Ensure we have sensible defaults in case they are not in the config file
	setDefaults()

	return nil
}

// setDefaults sets default configuration values based on the environment
func setDefaults() {
	// Check the current environment (default is development)
	env := viper.GetString("ENV")
	if env == "" {
		env = "development"
		viper.Set("ENV", env)
	}

	// Set defaults for development and production environments
	if env == "development" {
		viper.SetDefault("relay_backend_url", "http://localhost:9002")
		viper.SetDefault("allowed_origin", "http://localhost:3000")
		viper.SetDefault("wallet_db_path", "./dev_wallet.db")
		viper.SetDefault("log_level", "debug")
	} else if env == "production" {
		viper.SetDefault("relay_backend_url", "https://my-production-backend.com")
		viper.SetDefault("allowed_origin", "https://my-production-site.com")
		viper.SetDefault("wallet_db_path", "/var/lib/bitcoin-wallet/wallet.db")
		viper.SetDefault("log_level", "info")
	}

	// Common defaults for both environments
	viper.SetDefault("user_pubkey", "")
	viper.SetDefault("network", "mainnet") // or "testnet" or "regtest"
	viper.SetDefault("wallet_name", "")
	viper.SetDefault("rpc_server", "127.0.0.1:8332")
	viper.SetDefault("rpc_user", "rpcuser")
	viper.SetDefault("rpc_password", "rpcpassword")
	viper.SetDefault("use_tor", false)
	viper.SetDefault("tor_proxy", "127.0.0.1:9050")
	viper.SetDefault("max_peers", 125)
	viper.SetDefault("min_peers", 3)
	viper.SetDefault("api_port", 9003)
	viper.SetDefault("use_https", false)
	viper.SetDefault("cert_file", "server.crt")
	viper.SetDefault("key_file", "server.key")
	viper.SetDefault("fee_per_kb", 1000)    // in satoshis
	viper.SetDefault("dust_limit", 546)     // in satoshis
	viper.SetDefault("tx_max_size", 100000) // in bytes
	viper.SetDefault("address_gap_limit", 20)
	viper.SetDefault("sync_interval", "10m")
	viper.SetDefault("backup_interval", "24h")
	viper.SetDefault("backup_path", "./wallet_backup")
	viper.SetDefault("wallet_dir", "./wallets")
	viper.SetDefault("jwt_keys_dir", "./jwtkeys")
	viper.SetDefault("wallet_api_key", "")
	viper.SetDefault("server_mode", true)
	viper.SetDefault("relay_wallet_set", false)

	// Example peers for both environments
	viper.SetDefault("add_peers", []string{
		"seed.bitcoin.sipa.be:8333",
		"dnsseed.bluematt.me:8333",
		"seed.bitnodes.io:8333",
		"dnsseed.bitcoin.dashjr.org:8333",
		"seed.bitcoinstats.com:8333",
		"seed.bitcoin.jonasschnelli.ch:8333",
		"seed.btc.petertodd.org:8333",
		"seed.bitcoin.sprovoost.nl:8333",
		"dnsseed.emzy.de:8333",
		"bitcoin.dynDNS.us:8333",
		"seed.bitcoin.wiz.biz:8333",
		"bitcoin.lukechilds.co:8333",
		"dnsseed.bitcoinrelay.net:8333",
		"seed.bitcoin.meteo.network:8333",
		"seed.bitcoin.locha.io:8333",
		"seed.bitdevsnet.io:8333",
		"seed.bitcoinstats.com:8333",
		"seed.btc.altcoinsfoundation.com:8333",
		"seed.bitcoin-seeders.net:8333",
		"btcd-mainnet.lightning.computer:8333",
		"neutrino.bitcoin.kndx.dev:8333",
	})
}

// createDefaultConfig creates a new configuration file if it doesn't exist
func createDefaultConfig() error {
	setDefaults()

	// Write the default configuration to a file
	err := viper.SafeWriteConfig()
	if err != nil {
		if os.IsExist(err) {
			// If the config already exists, attempt to overwrite it
			err = viper.WriteConfig()
			if err != nil {
				return fmt.Errorf("error writing config file: %w", err)
			}
		} else {
			return fmt.Errorf("error creating config file: %w", err)
		}
	}

	fmt.Println("Created default configuration file")
	return nil
}
