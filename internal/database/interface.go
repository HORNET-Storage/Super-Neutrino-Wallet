package walletstatedb

import (
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet"
)

// DatabaseType represents the type of database backend to use
type DatabaseType string

const (
	// DBTypeGraviton represents the Graviton database backend
	DBTypeGraviton DatabaseType = "graviton"
	// DBTypeSQLite represents the SQLite database backend
	DBTypeSQLite DatabaseType = "sqlite"
)

// DBBackend is the global variable that holds the active database backend
var DBBackend DatabaseType = DBTypeGraviton

// SetDatabaseBackend sets the database backend type
func SetDatabaseBackend(dbType DatabaseType) {
	DBBackend = dbType
}

// InitializeDatabase initializes the database using the specified backend
func InitializeDatabase(dbPath string) error {
	switch DBBackend {
	case DBTypeGraviton:
		return InitDB(dbPath)
	case DBTypeSQLite:
		return InitSQLiteDB(dbPath)
	default:
		return InitDB(dbPath)
	}
}

// DatabaseInterface defines the interface for database operations
// This allows us to switch between Graviton and SQLite implementations
type DatabaseInterface interface {
	// Address operations
	SaveAddress(address Address) error
	GetAddresses() ([]Address, error)
	GetUnusedAddress() (*Address, error)
	MarkAddressAsUsed(address string, blockHeight uint32) error
	AllocateAddress() (*Address, error)
	GetLastAddressIndex() (int, error)
	
	// Block height operations
	SaveBlockHeight(height int32) error
	GetBlockHeight() (int32, error)
	
	// Transaction operations
	SaveRawTransaction(txHash string, rawTx []byte) error
	GetRawTransaction(txHash string) ([]byte, error)
	SaveTransactionToDB(tx *wire.MsgTx) (chainhash.Hash, error)
	SaveNewTransaction(tx *Transaction) error
	GetUnsentTransactions() ([]Transaction, error)
	ClearUnsentTransactions() error
	TransactionExists(txID string, vout uint32) (bool, error)
	
	// Challenge operations
	SaveChallenge(challenge Challenge) error
	GetChallenge(hash string) (*Challenge, error)
	MarkChallengeAsUsed(hash string) error
	ExpireOldChallenges() error
	
	// Address generation
	GenerateNewAddresses(w *wallet.Wallet, count int) error
	EnsureMinimumAvailableAddresses(w *wallet.Wallet) error
	
	// Address retrieval
	RetrieveAddresses() ([]btcutil.Address, []btcutil.Address, error)
	
	// Last scanned block height
	SetLastScannedBlockHeight(height int32) error
	GetLastScannedBlockHeight() (int32, error)
	UpdateLastScannedBlockHeight(height int32) error
}

// Common database operations that dispatch to the active backend
// These functions will route to either the Graviton or SQLite implementation
// based on the DBBackend variable

// SaveAddress saves an address to the database
func SaveAddressWithType(addrType string, address Address) error {
	switch DBBackend {
	case DBTypeSQLite:
		return SaveAddressToSQLite(addrType, address)
	default:
		// For Graviton, we need to determine the tree based on addrType
		ss, err := Store.LoadSnapshot(0)
		if err != nil {
			return err
		}
		
		var treeName string
		if addrType == "receive" {
			treeName = "receive_addresses"
		} else if addrType == "change" {
			treeName = "change_addresses"
		} else {
			return fmt.Errorf("unknown address type: %s", addrType)
		}
		
		tree, err := ss.GetTree(treeName)
		if err != nil {
			return err
		}
		
		return SaveAddress(tree, address)
	}
}

// GetAddressesWithType retrieves addresses of a specific type
func GetAddressesWithType(addrType string) ([]Address, error) {
	switch DBBackend {
	case DBTypeSQLite:
		return GetAddressesFromSQLite(addrType)
	default:
		// For Graviton, we need to determine the tree based on addrType
		ss, err := Store.LoadSnapshot(0)
		if err != nil {
			return nil, err
		}
		
		var treeName string
		if addrType == "receive" {
			treeName = "receive_addresses"
		} else if addrType == "change" {
			treeName = "change_addresses"
		} else {
			return nil, fmt.Errorf("unknown address type: %s", addrType)
		}
		
		tree, err := ss.GetTree(treeName)
		if err != nil {
			return nil, err
		}
		
		return GetAddresses(tree)
	}
}

// GetLastAddressIndexWithType gets the last address index for a specific type
func GetLastAddressIndexWithType(addrType string) (int, error) {
	switch DBBackend {
	case DBTypeSQLite:
		return GetLastAddressIndexFromSQLite(addrType)
	default:
		// For Graviton, we need to determine the tree based on addrType
		ss, err := Store.LoadSnapshot(0)
		if err != nil {
			return 0, err
		}
		
		var treeName string
		if addrType == "receive" {
			treeName = "receive_addresses"
		} else if addrType == "change" {
			treeName = "change_addresses"
		} else {
			return 0, fmt.Errorf("unknown address type: %s", addrType)
		}
		
		tree, err := ss.GetTree(treeName)
		if err != nil {
			return 0, err
		}
		
		return GetLastAddressIndex(tree)
	}
}