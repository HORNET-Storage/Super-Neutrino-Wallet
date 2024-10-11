package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/logger"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ripemd160"
	"golang.org/x/crypto/scrypt"
)

// deriveKeyFromPath derives the extended key from the given path
func DeriveKeyFromPath(rootKey *hdkeychain.ExtendedKey, path string) (*hdkeychain.ExtendedKey, error) {
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
func GetMasterFingerprint(rootKey *hdkeychain.ExtendedKey) (uint32, error) {
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
func GetExtendedPubKey(extendedKey *hdkeychain.ExtendedKey, version []byte) (string, error) {
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

func EstimateBlockHeight(targetDate time.Time) int32 {
	genesisDate := time.Date(2009, time.January, 3, 18, 15, 5, 0, time.UTC)
	daysSinceGenesis := targetDate.Sub(genesisDate).Hours() / 24
	estimatedHeight := int32(daysSinceGenesis * 144)
	return estimatedHeight
}

func GracefulShutdown() error {
	time.Sleep(1 * time.Second)
	fmt.Println("Shutdown complete. Goodbye!")
	err := SetWalletLive(false)
	if err != nil {
		log.Printf("Error setting wallet live state: %v", err)
	}

	err = SetWalletSync(false)
	if err != nil {
		log.Printf("Error setting wallet sync state: %v", err)
	}

	defer logger.Cleanup() // Ensure the log file is closed

	time.Sleep(2 * time.Second) // Give user time to read the message
	os.Exit(0)
	return nil
}

func SetWalletSync(synced bool) error {
	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}

	if !viper.IsSet("wallet_synced") {
		viper.Set("wallet_synced", synced)
	} else {
		viper.Set("wallet_synced", synced)
	}

	err = viper.WriteConfig()
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

func SetWalletLive(live bool) error {
	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}

	if !viper.IsSet("wallet_live") {
		viper.Set("wallet_live", live)
	} else {
		viper.Set("wallet_live", live)
	}

	err = viper.WriteConfig()
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

func Decrypt(ciphertext string, password string) (string, error) {
	parts := strings.Split(ciphertext, ":")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid ciphertext format")
	}

	salt, _ := base64.StdEncoding.DecodeString(parts[0])
	iv, _ := base64.StdEncoding.DecodeString(parts[1])
	encryptedData, _ := base64.StdEncoding.DecodeString(parts[2])

	key, _ := deriveKey(password, salt)
	block, _ := aes.NewCipher(key)
	aesgcm, _ := cipher.NewGCM(block)
	plaintext, err := aesgcm.Open(nil, iv, encryptedData, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func deriveKey(password string, salt []byte) ([]byte, []byte) {
	if salt == nil {
		salt = make([]byte, 32)
		if _, err := rand.Read(salt); err != nil {
			panic(err)
		}
	}

	key, err := scrypt.Key([]byte(password), salt, 1<<15, 8, 1, 32)
	if err != nil {
		panic(err)
	}

	return key, salt
}

func GenerateSelfSignedCert() error {
	// Define the file paths for the certificate and key
	certFile := "server.crt"
	keyFile := "server.key"

	// Check if the certificate and key already exist
	if _, err := os.Stat(certFile); err == nil {
		log.Println("Certificate already exists, skipping generation.")
		return nil
	}

	log.Println("Generating a new self-signed certificate...")

	// Create the command to generate a private key and a self-signed certificate using OpenSSL
	cmd := exec.Command("openssl", "req", "-newkey", "rsa:2048", "-nodes", "-keyout", keyFile, "-x509", "-days", "365",
		"-out", certFile, "-subj", "/CN=localhost/O=Localhost Development", "-addext", "subjectAltName=DNS:localhost")

	// Run the OpenSSL command
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to generate self-signed certificate: %w, output: %s", err, output)
	}

	log.Println("Self-signed certificate generated successfully.")
	return nil
}

func TrustCertificate(certFile string) error {
	osType := runtime.GOOS // Detect the operating system
	switch osType {
	case "darwin":
		return TrustCertificateMacOS(certFile)
	case "linux":
		return TrustCertificateLinux(certFile)
	case "windows":
		return TrustCertificateWindows(certFile)
	default:
		return fmt.Errorf("unsupported operating system: %s", osType)
	}
}

// macOS-specific command to trust the certificate
func TrustCertificateMacOS(certFile string) error {
	log.Println("Trusting certificate on macOS...")

	cmd := exec.Command("sudo", "security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", certFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to trust certificate on macOS: %w, output: %s", err, output)
	}

	log.Println("Certificate trusted successfully on macOS.")
	return nil
}

// Linux-specific command to trust the certificate
func TrustCertificateLinux(certFile string) error {
	log.Println("Trusting certificate on Linux...")

	cmd := exec.Command("sudo", "cp", certFile, "/usr/local/share/ca-certificates/")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to copy certificate to trusted directory on Linux: %w, output: %s", err, output)
	}

	// Update CA certificates
	cmd = exec.Command("sudo", "update-ca-certificates")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update CA certificates on Linux: %w, output: %s", err, output)
	}

	log.Println("Certificate trusted successfully on Linux.")
	return nil
}

// Windows-specific command to trust the certificate (requires CertUtil)
func TrustCertificateWindows(certFile string) error {
	log.Println("Trusting certificate on Windows...")

	cmd := exec.Command("certutil", "-addstore", "Root", certFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to trust certificate on Windows: %w, output: %s", err, output)
	}

	log.Println("Certificate trusted successfully on Windows.")
	return nil
}

// deleteWalletFiles deletes all wallet-related files and directories for a given wallet name.
func DeleteWalletFiles(walletName string) error {
	// Get the base directories from the configuration
	baseDir := viper.GetString("base_dir")                 // Base directory for general wallet-related files
	walletDir := viper.GetString("wallet_dir")             // Directory containing .env files
	neutrinoDbDir := filepath.Join(baseDir, "neutrino_db") // Neutrino database directory
	jwtKeysDir := filepath.Join(baseDir, "jwtkeys")        // JWT keys directory

	// Paths for wallet-specific files and directories
	envFile := filepath.Join(walletDir, walletName+".env")                                     // .env file
	neutrinoWalletDir := filepath.Join(neutrinoDbDir, fmt.Sprintf("%s_wallet", walletName))    // Neutrino wallet directory
	gravitonDbFile := filepath.Join(baseDir, fmt.Sprintf("%s_wallet_graviton.db", walletName)) // Graviton DB file
	jwtKeyDir := filepath.Join(jwtKeysDir, walletName)                                         // JWT key directory

	// Delete the wallet .env file
	if err := os.Remove(envFile); err != nil {
		log.Printf("Failed to delete .env file: %v", err) // Continue even if env file removal fails
	} else {
		log.Printf("Successfully deleted .env file: %s", envFile)
	}

	// Delete the Neutrino wallet directory and its contents
	if err := os.RemoveAll(neutrinoWalletDir); err != nil {
		log.Printf("Failed to delete Neutrino wallet directory: %v", err)
	} else {
		log.Printf("Successfully deleted Neutrino wallet directory: %s", neutrinoWalletDir)
	}

	// Delete the Graviton DB file
	if err := os.RemoveAll(gravitonDbFile); err != nil {
		log.Printf("Failed to delete Graviton DB file: %v", err)
	} else {
		log.Printf("Successfully deleted Graviton DB file or directory: %s", gravitonDbFile)
	}

	// Delete the JWT key directory and its contents
	if err := os.RemoveAll(jwtKeyDir); err != nil {
		log.Printf("Failed to delete JWT key directory: %v", err)
	} else {
		log.Printf("Successfully deleted JWT key directory: %s", jwtKeyDir)
	}

	return nil
}

func CleanupExistingData(neutrinoDBPath, walletDBPath string) error {
	if err := os.RemoveAll(neutrinoDBPath); err != nil {
		return fmt.Errorf("failed to remove Neutrino database directory: %v", err)
	}
	if err := os.MkdirAll(neutrinoDBPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to recreate Neutrino database directory: %v", err)
	}
	walletDir := filepath.Dir(walletDBPath)
	if err := os.MkdirAll(walletDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to recreate wallet directory: %v", err)
	}
	return nil
}

func Encrypt(plaintext string, password string) string {
	key, salt := deriveKey(password, nil)
	iv := make([]byte, 12)
	rand.Read(iv)

	block, _ := aes.NewCipher(key)
	aesgcm, _ := cipher.NewGCM(block)
	ciphertext := aesgcm.Seal(nil, iv, []byte(plaintext), nil)

	return base64.StdEncoding.EncodeToString(salt) + ":" +
		base64.StdEncoding.EncodeToString(iv) + ":" +
		base64.StdEncoding.EncodeToString(ciphertext)
}
