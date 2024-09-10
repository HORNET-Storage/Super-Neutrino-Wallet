package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/crypto/scrypt"
)

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
func decrypt(ciphertext string, password string) (string, error) {
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
