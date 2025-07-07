# Super Neutrino - BTC Desktop Wallet

A secure and feature-rich Bitcoin wallet application built with Go. While originally designed to support payment for subscriptions in the [HORNETS Nostr Relay](https://github.com/HORNET-Storage/HORNETS-Nostr-Relay) ecosystem, it can be used as a standalone Bitcoin wallet. Use it via terminal, the [HORNETS Relay Panel](https://github.com/HORNET-Storage/HORNETS-Relay-Panel), or the [Nestr client](https://github.com/HORNET-Storage/nestr).

## Table of Contents

- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Building](#building)
- [Usage](#usage)
- [Configuration](#configuration)
- [Security](#security)
- [Contributing](#contributing)
- [License](#license)

## Features

- Create new wallets or import existing ones
- Secure storage of wallet data
- Support for mainnet
- Transaction creation and signing
- Balance checking
- Address generation and management
- RBF (Replace-By-Fee) support
- Integration with Neutrino for lightweight node functionality
- JWT-based API authentication

## Prerequisites

- Go 1.16 or higher

## Installation

1. Clone the repository:
   ```
   git clone https://github.com/HORNET-Storage/Super-Neutrino-Wallet.git
   cd Super-Neutrino-wallet
   ```

2. Install dependencies:
   ```
   go mod download
   ```

## Building

You can build the Super Neutrino Wallet using the provided `build.sh` script:

1. Make the script executable:
   ```
   chmod +x build.sh
   ```

2. Run the build script:
   ```
   ./build.sh
   ```

This will create an executable named `SN-wallet` in your current directory.

## Usage

To start the Super Neutrino Wallet application, simply run:

```
./SN-wallet
```

You will be presented with the following options:

```
Bitcoin Wallet Manager
1. Create a new wallet
2. Import an existing wallet
3. Login to an existing wallet
4. Delete a wallet
5. Exit

Enter your choice (1, 2, 3, 4, or 5):
```

Navigate through the application by entering the number corresponding to your desired action:

1. **Create a new wallet**: Set up a brand new Bitcoin wallet. You'll be guided through the process of generating a new seed phrase and securing your wallet.

2. **Import an existing wallet**: Restore a wallet using an existing seed phrase. Use this option if you're moving your wallet to a new device or restoring from a backup.

3. **Login to an existing wallet**: Access a wallet you've previously created or imported on this device.

4. **Delete a wallet**: Select and permanently delete a wallet from the device. **Caution**: Deleting a wallet will remove its data from the device, but this action does not delete the wallet from the Bitcoin network. Ensure you have securely backed up the seed phrase before proceeding, as deleting without a backup means you permanently lose access to the wallet and any funds within it.

5. **Exit**: Close the application.

Follow the on-screen prompts for each option. Make sure to securely store any seed phrases or passwords you create or use.

**Important Note:** First-time blockchain syncing can take considerable time depending on your internet connection, system resources, and connected peers. The wallet needs to sync with the Bitcoin network before it can display accurate balance and transaction information.

## Configuration

The wallet uses a configuration file (`config.json`) for various settings. This wallet is specifically designed to support payment for subscriptions in the [HORNETS Nostr Relay](https://github.com/HORNET-Storage/HORNETS-Nostr-Relay) project.

### Setting Up config.json

The `config.json` file is automatically generated when you first start the wallet and go through the setup process. After the initial setup:

1. Locate the `config.json` file in the root directory of the project.
2. Open it with a text editor.
3. Configure the essential fields as described below.

**Required Configuration Fields:**

- `user_pubkey`: Your Nostr public key (same as used in the relay panel)
- `wallet_api_key`: API key from the relay's config.yaml
- `wallet_name`: Must be set to "default" for relay integration
- `network`: Bitcoin network ("mainnet")
- `api_port`: Port for the API server (default: 9003)

### Getting the API Key from HORNETS Relay

The `wallet_api_key` must be obtained from your HORNETS Nostr Relay configuration:

1. **Locate your relay's config.yaml file**
2. **Find the wallet section:**
   ```yaml
   wallet:
     key: c3eb99c13ca2a2f93b9fbcdbb0666bd5faf4595bd07a73f358b330eda86da658
     name: default
     url: http://localhost:9003
   ```
3. **Copy the `key` value** and use it as your `wallet_api_key` in the wallet's config.json

### Setting Up Your Public Key

The `user_pubkey` should be your Nostr public key that you use with:
- The [HORNETS Relay Panel](https://github.com/HORNET-Storage/HORNETS-Relay-Panel)
- Your NIP-07 compatible browser extension (like Alby, nos2x, etc.)

To get your public key:
1. Use a NIP-07 compatible browser extension
2. Your extension provides `window.nostr.getPublicKey()` method
3. Use the same public key in both the relay panel and this wallet

**Complete Configuration Example:**

```json
{
  "add_peers": [
    "bitcoin.aranguren.org:8333",
    "bitcoin.bs.ts.net:8333"
  ],
  "address_gap_limit": 20,
  "allowed_origin": "http://localhost:3000",
  "api_port": 9003,
  "backup_interval": "24h",
  "backup_path": "./wallet_backup",
  "base_dir": "/path/to/your/wallet/directory",
  "cert_file": "server.crt",
  "dust_limit": 546,
  "env": "development",
  "fee_per_kb": 1000,
  "jwt_keys_dir": "./jwtkeys",
  "key_file": "server.key",
  "log_level": "debug",
  "max_peers": 125,
  "min_peers": 3,
  "network": "mainnet",
  "relay_backend_url": "http://localhost:9002",
  "rpc_password": "rpcpassword",
  "rpc_server": "127.0.0.1:8332",
  "rpc_user": "rpcuser",
  "server_mode": true,
  "sync_interval": "10m",
  "tor_proxy": "127.0.0.1:9050",
  "tx_max_size": 100000,
  "use_https": false,
  "use_tor": false,
  "user_pubkey": "your_nostr_public_key_here",
  "wallet_api_key": "key_from_relay_config_yaml",
  "wallet_db_path": "./dev_wallet.db",
  "wallet_dir": "./wallets",
  "wallet_name": "default"
}
```

**Important Notes:**
- The `wallet_name` MUST be set to "default" for the relay to properly identify the wallet
- Ensure the `api_port` matches the port specified in your relay's config.yaml
- The `user_pubkey` should be the same public key you use for signing events in the relay panel

## Security

- The wallet uses BIP39 for seed phrase generation
- Sensitive data (seed phrases, private keys) are encrypted before storage
- JWT is used for API authentication

Always ensure you're running the latest version of the wallet and keep your system updated.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgements

- [btcsuite](https://github.com/btcsuite) for their Bitcoin libraries
- [Neutrino](https://github.com/lightninglabs/neutrino) for lightweight node functionality
- All contributors who have helped shape Super Neutrino
