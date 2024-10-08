package operations

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/api"
	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/Maphikza/btc-wallet-btcsuite.git/internal/wallet/utils"
	"github.com/joho/godotenv"
)

var (
	engaged     bool
	exitMutex   sync.Mutex
	exiting     bool
	transacting bool
	walletDir   = "./wallets"
)

type WalletServer struct {
	API *api.API
}

func (s *WalletServer) HandleCommand(command string) error {
	if s.API.HttpMode {
		return fmt.Errorf("terminal commands are not available in HTTP mode")
	}

	switch command {
	case "tx":
		return s.PerformTransaction()
	case "exit":
		return s.ExitWallet()
	case "seed-view":
		return s.ViewSeedPhrase()
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func ListenForUserCommands(commandChannel chan<- string) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		if !engaged {
			fmt.Println("\nAvailable commands:")
			fmt.Println("- 'tx': Enter transaction context")
			fmt.Println("- 'seed-view': View seed phrase")
			fmt.Println("- 'exit': Close the wallet")
			fmt.Print("\nEnter command: ")
			scanner.Scan()
			command := strings.TrimSpace(scanner.Text())

			commandChannel <- command
		}
		time.Sleep(100 * time.Millisecond) // Add a small delay to reduce CPU usage
	}
}

func (s *WalletServer) ViewSeedPhrase() error {
	engaged = true
	defer func() { engaged = false }()

	return ViewSeedPhrase()
}

func (s *WalletServer) HandleGetReceiveAddresses() (interface{}, error) {
	// Retrieve receive and change addresses
	receiveAddresses, _, err := walletstatedb.RetrieveAddresses()
	if err != nil {
		return nil, err
	}

	// Convert []btcutil.Address to []string
	receiveAddressStrings := make([]string, len(receiveAddresses))
	for i, addr := range receiveAddresses {
		receiveAddressStrings[i] = addr.String() // Convert each btcutil.Address to its string representation
	}

	log.Printf("Receive addresses retrieved: %v\n", receiveAddressStrings)
	return map[string][]string{"addresses": receiveAddressStrings}, nil
}

func ViewSeedPhrase() error {
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

	seedPhrase, err := utils.Decrypt(encryptedSeedPhrase, password)
	if err != nil {
		return fmt.Errorf("error decrypting seed phrase: %v", err)
	}

	fmt.Println("Your seed phrase is:")
	fmt.Println(seedPhrase)
	fmt.Println("Please ensure you store this securely and never share it with anyone.")

	return nil
}
