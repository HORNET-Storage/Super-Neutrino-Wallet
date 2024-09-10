# Nestr Wallet

A secure and feature-rich Bitcoin wallet application built with Go.

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
   git clone https://github.com/Maphikza/Nestr-wallet.git
   cd Nestr-wallet
   ```

2. Install dependencies:
   ```
   go mod download
   ```

## Building

You can build the Nestr wallet using the provided `build.sh` script:

1. Make the script executable:
   ```
   chmod +x build.sh
   ```

2. Run the build script:
   ```
   ./build.sh
   ```

This will create an executable named `Nestr-wallet` in your current directory.

## Usage

To start the Nestr wallet application, simply run:

```
./Nestr-wallet
```

You will be presented with the following options:

```
Bitcoin Wallet Manager
1. Create a new wallet
2. Import an existing wallet
3. Login to an existing wallet
4. Exit

Enter your choice (1, 2, 3, or 4): 
```

Navigate through the application by entering the number corresponding to your desired action:

1. **Create a new wallet**: Set up a brand new Bitcoin wallet. You'll be guided through the process of generating a new seed phrase and securing your wallet.

2. **Import an existing wallet**: Restore a wallet using an existing seed phrase. Use this option if you're moving your wallet to a new device or restoring from a backup.

3. **Login to an existing wallet**: Access a wallet you've previously created or imported on this device.

4. **Exit**: Close the application.

Follow the on-screen prompts for each option. Make sure to securely store any seed phrases or passwords you create or use.

## Configuration

The wallet uses a configuration file (`config.json`) for various settings. Before using the wallet, you must set up this configuration file correctly.

### Setting Up config.json

1. Locate the `config.json` file in the root directory of the project.
2. Open it with a text editor.
3. Find the `user_pubkey` field and set your public key. This is crucial for authentication and authorization.

Example `config.json`:

```json
{
  "user_pubkey": "your_public_key_here"
}
```

Replace `"your_public_key_here"` with your actual public key.

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
- All contributors who have helped shape Nestr Wallet
