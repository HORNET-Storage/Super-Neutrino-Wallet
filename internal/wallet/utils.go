package wallet

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	transaction "github.com/Maphikza/btc-wallet-btcsuite.git/lib/transaction"
	"github.com/Maphikza/btc-wallet-btcsuite.git/lib/utils"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/ripemd160"
)

// deriveKeyFromPath derives the extended key from the given path
func deriveKeyFromPath(rootKey *hdkeychain.ExtendedKey, path string) (*hdkeychain.ExtendedKey, error) {
	parts := strings.Split(path, "/")
	key := rootKey
	for _, part := range parts {
		var index uint32
		var err error
		if strings.HasSuffix(part, "'") {
			index64, err := strconv.ParseUint(part[:len(part)-1], 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid path component %s: %v", part, err)
			}
			index = hdkeychain.HardenedKeyStart + uint32(index64)
		} else {
			index64, err := strconv.ParseUint(part, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid path component %s: %v", part, err)
			}
			index = uint32(index64)
		}
		key, err = key.Derive(index)
		if err != nil {
			return nil, fmt.Errorf("failed to derive key: %v", err)
		}
	}
	return key, nil
}

// getMasterFingerprint calculates the master fingerprint from the root key
func getMasterFingerprint(rootKey *hdkeychain.ExtendedKey) (uint32, error) {
	pubKey, err := rootKey.ECPubKey()
	if err != nil {
		return 0, fmt.Errorf("failed to get public key from root key: %v", err)
	}

	sha := sha256.New()
	_, err = sha.Write(pubKey.SerializeCompressed())
	if err != nil {
		return 0, fmt.Errorf("failed to write sha256: %v", err)
	}
	hash160 := ripemd160.New()
	_, err = hash160.Write(sha.Sum(nil))
	if err != nil {
		return 0, fmt.Errorf("failed to write ripemd160: %v", err)
	}
	fingerprint := hash160.Sum(nil)[:4]

	return binary.BigEndian.Uint32(fingerprint), nil
}

// getExtendedPubKey converts an extended key to its string representation with the given version bytes
func getExtendedPubKey(extendedKey *hdkeychain.ExtendedKey, version []byte) (string, error) {
	neuteredKey, err := extendedKey.Neuter()
	if err != nil {
		return "", err
	}
	clonedKey, err := neuteredKey.CloneWithVersion(version)
	if err != nil {
		return "", err
	}
	return clonedKey.String(), nil
}

// Function to hash a file
func hashFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("error opening file: %v", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("error hashing file: %v", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func estimateBlockHeight(targetDate time.Time) int32 {
	genesisDate := time.Date(2009, time.January, 3, 18, 15, 5, 0, time.UTC)
	daysSinceGenesis := targetDate.Sub(genesisDate).Hours() / 24
	estimatedHeight := int32(daysSinceGenesis * 144)
	return estimatedHeight
}

func isBirthdayToday(birthday time.Time) bool {
	today := time.Now()
	return birthday.Month() == today.Month() &&
		birthday.Day() == today.Day() &&
		birthday.Year() == today.Year()
}

func viewSeedPhrase() error {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("Enter the name of the wallet to view seed phrase: ")
	scanner.Scan()
	walletName := strings.TrimSpace(scanner.Text())

	envFile := filepath.Join(walletDir, walletName+".env")
	err := godotenv.Load(envFile)
	if err != nil {
		return fmt.Errorf("error loading wallet file: %v", err)
	}

	encryptedSeedPhrase := os.Getenv("ENCRYPTED_SEED_PHRASE")
	if encryptedSeedPhrase == "" {
		return fmt.Errorf("encrypted seed phrase not found")
	}

	fmt.Print("Enter your wallet password: ")
	reader := bufio.NewReader(os.Stdin)
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	seedPhrase, err := decrypt(encryptedSeedPhrase, password)
	if err != nil {
		return fmt.Errorf("error decrypting seed phrase: %v", err)
	}

	fmt.Println("Your seed phrase is:")
	fmt.Println(seedPhrase)
	fmt.Println("Please ensure you store this securely and never share it with anyone.")

	return nil
}

func gracefulShutdown() error {
	time.Sleep(1 * time.Second)
	fmt.Println("Shutdown complete. Goodbye!")
	err := setWalletLive(false)
	if err != nil {
		log.Printf("Error setting wallet live state: %v", err)
	}

	err = setWalletSync(false)
	if err != nil {
		log.Printf("Error setting wallet sync state: %v", err)
	}

	time.Sleep(2 * time.Second) // Give user time to read the message
	os.Exit(0)
	return nil
}

func handleAddressGeneration(w *wallet.Wallet, chainClient *chain.NeutrinoClient, needsAddresses, freshWallet bool) error {
	var numberofAddr int
	if needsAddresses {
		numberofAddr = 30
		err := generateInitialAddresses(w, chainClient, numberofAddr)
		if err != nil {
			return fmt.Errorf("error generating initial addresses: %s", err)
		}
	} else if freshWallet {
		numberofAddr = 1
		err := generateInitialAddresses(w, chainClient, numberofAddr)
		if err != nil {
			return fmt.Errorf("error generating initial addresses: %s", err)
		}
	} else {
		log.Println("Using existing addresses from the database")
	}

	return nil
}

func generateInitialAddresses(w *wallet.Wallet, chainClient *chain.NeutrinoClient, numAddresses int) error {
	const maxRetries = 2
	var receiveAddresses, changeAddresses []btcutil.Address
	var err error

	for i := 0; i < maxRetries; i++ {
		log.Printf("Attempt %d: Generating initial addresses", i+1)
		receiveAddresses, changeAddresses, err = utils.GenerateAndSaveAddresses(w, numAddresses)
		if err == nil {
			log.Printf("Successfully generated and saved %d receive addresses and %d change addresses", len(receiveAddresses), len(changeAddresses))
			break
		}
		log.Printf("Error generating addresses: %v", err)
		if i < maxRetries-1 {
			log.Println("Retrying in 5 seconds...")
			time.Sleep(5 * time.Second)
		}
	}

	if err != nil {
		log.Printf("Failed to generate addresses after %d attempts", maxRetries)
		return fmt.Errorf("failed to generate addresses after %d attempts, with error: %s", maxRetries, err)
	} else {
		utils.PrintAddresses("Receive", receiveAddresses)
		utils.PrintAddresses("Change", changeAddresses)
	}

	_, chainClientBestblock, err := chainClient.GetBestBlock()
	if err != nil {
		return fmt.Errorf("error getting chain client best block: %v", err)
	} else {
		log.Printf("Chain client best block: %d", chainClientBestblock)
		err = walletstatedb.SetLastScannedBlockHeight(chainClientBestblock)
		if err != nil {
			return fmt.Errorf("error setting initial last scanned block height: %v", err)
		} else {
			log.Printf("Initial last scanned block height set to %d", chainClientBestblock)
		}
	}

	return nil
}

func generateRandomPassphrase(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func GetWalletBalance(w *wallet.Wallet) (int64, error) {
	balance, err := w.CalculateBalance(1) // Use 1 confirmation
	if err != nil {
		return 0, fmt.Errorf("error calculating balance: %v", err)
	}
	return int64(balance), nil
}

func EstimateTransactionSize(w *wallet.Wallet, spendAmount int64, recipientAddress string, feeRate int) (int, error) {
	return transaction.HttpCalculateTransactionSize(w, spendAmount, recipientAddress, feeRate)
}

func GetTransactionHistory(w *wallet.Wallet, walletName string) ([]map[string]interface{}, error) {
	transactions, err := w.ListAllTransactions()
	if err != nil {
		return nil, fmt.Errorf("error listing transactions: %v", err)
	}

	var result []map[string]interface{}

	for _, tx := range transactions {
		transaction := map[string]interface{}{
			"txid":   tx.TxID,
			"date":   time.Unix(tx.Time, 0).Format(time.RFC3339),
			"amount": fmt.Sprintf("%.8f", tx.Amount),
		}
		result = append(result, transaction)
	}

	return result, nil
}
