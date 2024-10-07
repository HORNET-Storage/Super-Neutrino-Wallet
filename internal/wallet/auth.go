package wallet

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/crypto/scrypt"

	"github.com/spf13/viper"
)

func OpenAndloadWallet(reader *bufio.Reader, baseDir string) error {
	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}

	// List all available wallets
	wallets, err := listWallets()
	if err != nil {
		return fmt.Errorf("error listing wallets: %v", err)
	}

	if len(wallets) == 0 {
		fmt.Println("No wallets found. Please create a new wallet first.")
		return errors.New("no wallets found. Please create a new wallet first")
	}

	// Display the available wallets
	fmt.Println("Available wallets:")
	for i, wallet := range wallets {
		fmt.Printf("%d. %s\n", i+1, wallet)
	}

	// Prompt the user to select a wallet
	var choice int
	for {
		fmt.Print("Enter the number of the wallet you want to login to: ")
		_, err := fmt.Fscanf(reader, "%d\n", &choice)
		if err == nil && choice > 0 && choice <= len(wallets) {
			break
		} else {
			fmt.Println("Invalid choice. Please try again.")
		}
	}

	// Get the selected wallet name
	walletName := wallets[choice-1]

	// Load the wallet data
	seedPhrase, publicPass, privatePass, birthdate, err := loadWallet(walletName)
	if err != nil {
		return fmt.Errorf("error loading wallet: %v", err)
	}

	var serverMode bool
	if !viper.IsSet("relay_wallet_set") {
		viper.Set("relay_wallet_set", false)
	}

	if !viper.IsSet("wallet_name") {
		viper.Set("wallet_name", "")
	}

	err = viper.WriteConfig()
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	// Check if the wallet is set to connect to the relay (i.e., "panel" mode)
	if viper.GetBool("relay_wallet_set") && viper.GetString("wallet_name") == walletName {
		log.Println("This is a relay wallet.")
		viper.Set("server_mode", true)
	}

	// If serverMode is true, ask if the user wants to use terminal mode
	if viper.GetBool("server_mode") && viper.GetString("wallet_name") == walletName {
		fmt.Print("This wallet is connected to the panel. If you use it in terminal/cli mode, it will not be connected to the panel.\nDo you want to use the wallet in terminal/CLI mode? (y/n): ")
		cliChoice, _ := reader.ReadString('\n')
		cliChoice = strings.TrimSpace(strings.ToLower(cliChoice))

		if cliChoice == "y" {
			fmt.Println("You are using the wallet in terminal mode. It will not be connected to the panel or relay.")
			viper.Set("server_mode", false)
		}
	}

	log.Println("Server mode: ", viper.GetBool("server_mode"))

	err = viper.WriteConfig()
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	// Proceed to start the wallet
	pubPass := []byte(publicPass)
	privPass := []byte(privatePass)

	if viper.IsSet("server_mode") {
		serverMode = viper.GetBool("server_mode")
	} else {
		serverMode = false
	}

	err = setWalletLive(true)
	if err != nil {
		log.Printf("Error setting wallet live state: %v", err)
	}

	err = StartWallet(seedPhrase, pubPass, privPass, baseDir, walletName, birthdate, serverMode)
	if err != nil {
		return fmt.Errorf("failed to start wallet: %v", err)
	}

	fmt.Printf("Wallet '%s' loaded successfully.\n", walletName)
	return nil
}

func OpenAndLoadWalletAPI(walletName, password string, baseDir string) error {
	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading viper config: %s", err.Error())
	}
	// Load the wallet data
	seedPhrase, publicPass, privatePass, birthdate, err := LoadWalletAPI(walletName, password)
	if err != nil {
		return fmt.Errorf("error loading wallet: %v", err)
	}

	// Check if the wallet is set to connect to the relay (i.e., "panel" mode)
	var serverMode bool
	if viper.GetBool("relay_wallet_set") && viper.GetString("wallet_name") == walletName {
		serverMode = true
	} else {
		serverMode = false
	}

	// Convert string passphrases to byte slices
	pubPass := []byte(publicPass)
	privPass := []byte(privatePass)

	err = setWalletLive(true)
	if err != nil {
		log.Printf("Error setting wallet live state: %v", err)
	}

	err = setWalletSync(false)
	if err != nil {
		log.Printf("Error setting wallet sync state: %v", err)
	}

	err = StartWallet(seedPhrase, pubPass, privPass, baseDir, walletName, birthdate, serverMode)
	if err != nil {
		return fmt.Errorf("failed to start wallet: %v", err)
	}

	fmt.Printf("Wallet '%s' loaded successfully.\n", walletName)
	return nil
}

func encrypt(plaintext string, password string) string {
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

// TODO: Remove if no longer necessary
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

// TODO: Remove if no longer necessary
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

func generateSelfSignedCert() error {
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

func trustCertificate(certFile string) error {
	osType := runtime.GOOS // Detect the operating system
	switch osType {
	case "darwin":
		return trustCertificateMacOS(certFile)
	case "linux":
		return trustCertificateLinux(certFile)
	case "windows":
		return trustCertificateWindows(certFile)
	default:
		return fmt.Errorf("unsupported operating system: %s", osType)
	}
}

// macOS-specific command to trust the certificate
func trustCertificateMacOS(certFile string) error {
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
func trustCertificateLinux(certFile string) error {
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
func trustCertificateWindows(certFile string) error {
	log.Println("Trusting certificate on Windows...")

	cmd := exec.Command("certutil", "-addstore", "Root", certFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to trust certificate on Windows: %w, output: %s", err, output)
	}

	log.Println("Certificate trusted successfully on Windows.")
	return nil
}
